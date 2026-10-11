package raft

import (
	"context"
	"log"
	"math/rand/v2"
	"sync"
	"time"
)

// Role is a node's Raft role.
type Role int

const (
	// Follower receives leader heartbeats and grants votes.
	Follower Role = iota
	// Candidate is running an election.
	Candidate
	// Leader serves client writes once replication lands.
	Leader
)

func (r Role) String() string {
	switch r {
	case Follower:
		return "follower"
	case Candidate:
		return "candidate"
	case Leader:
		return "leader"
	default:
		return "unknown"
	}
}

// Snapshot is a point-in-time view of a node's cluster state, safe to hand to
// status endpoints without holding the node's lock.
type Snapshot struct {
	Role   string
	Term   uint64
	Leader string
}

// Peer is another node in the cluster.
type Peer struct {
	ID  string
	URL string
}

// Config tunes election and heartbeat timing and identifies the node.
type Config struct {
	ID                 string
	Peers              []Peer // all other nodes, excluding self
	ElectionTimeoutMin time.Duration
	ElectionTimeoutMax time.Duration
	HeartbeatInterval  time.Duration
}

// DefaultConfig returns timings suited to local development and tests:
// 300–600ms election timeout, 100ms heartbeats. Real deployments would need
// larger values.
func DefaultConfig() Config {
	return Config{
		ElectionTimeoutMin: 300 * time.Millisecond,
		ElectionTimeoutMax: 600 * time.Millisecond,
		HeartbeatInterval:  100 * time.Millisecond,
	}
}

// Node is a single raft participant.
//
// Thread safety: the node's state (term, role, votedFor, leader, timers) is
// guarded by mu. Network calls happen outside the lock — RPC handlers take the
// lock on the receiving side, and calling out while holding our own lock could
// deadlock two nodes elected in the same term.
type Node struct {
	mu sync.Mutex

	id        string
	peers     []Peer
	cfg       Config
	transport Transport

	role     Role
	term     uint64
	votedFor string
	leaderID string

	// last log index/term at startup; the log itself arrives in phase 3.
	lastLogIndex uint64
	lastLogTerm  uint64

	lastActivity time.Time
	now          func() time.Time
	rand         *rand.Rand

	tick  *time.Ticker
	stopC chan struct{}
	wg    sync.WaitGroup
}

// NewNode creates a node. It does not start running until Start is called.
func NewNode(cfg Config, t Transport) *Node {
	return &Node{
		id:           cfg.ID,
		peers:        cfg.Peers,
		cfg:          cfg,
		transport:    t,
		role:         Follower,
		now:          time.Now,
		rand:         rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		lastActivity: time.Now(),
		stopC:        make(chan struct{}),
	}
}

// Start launches the node's run loop.
func (n *Node) Start() {
	n.mu.Lock()
	n.lastActivity = n.now()
	interval := n.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	n.tick = time.NewTicker(interval)
	n.mu.Unlock()

	n.wg.Add(1)
	go n.run()
}

// Stop terminates the run loop.
func (n *Node) Stop() {
	close(n.stopC)
	n.wg.Wait()
	if n.tick != nil {
		n.tick.Stop()
	}
}

// Snapshot returns a copy of the node's role/term/leader.
func (n *Node) Snapshot() Snapshot {
	n.mu.Lock()
	defer n.mu.Unlock()
	return Snapshot{Role: n.role.String(), Term: n.term, Leader: n.leaderID}
}

func (n *Node) run() {
	defer n.wg.Done()
	for {
		select {
		case <-n.tick.C:
			n.tickFunc()
		case <-n.stopC:
			return
		}
	}
}

func (n *Node) tickFunc() {
	now := n.now()
	n.mu.Lock()
	role := n.role
	deadline := n.electionTimeout()
	overdue := now.Sub(n.lastActivity) >= deadline
	n.mu.Unlock()

	switch role {
	case Follower, Candidate:
		if overdue {
			n.runElection()
		}
	case Leader:
		n.sendHeartbeats()
	}
}

// electionTimeout returns a random timeout in [min, max].
func (n *Node) electionTimeout() time.Duration {
	span := n.cfg.ElectionTimeoutMax - n.cfg.ElectionTimeoutMin
	if span <= 0 {
		return n.cfg.ElectionTimeoutMin
	}
	return n.cfg.ElectionTimeoutMin + time.Duration(n.rand.Int64N(int64(span)))
}

// majority is the number of votes/confirmations needed from a cluster of
// n.peers+1 nodes (self included).
func (n *Node) majority() int {
	return len(n.peers)/2 + 1
}

// runElection implements the candidate side of §5.2: bump the term, vote for
// self, solicit votes from every peer, become leader on a majority.
func (n *Node) runElection() {
	ctx := context.Background()

	n.mu.Lock()
	n.term++
	term := n.term
	n.role = Candidate
	n.votedFor = n.id
	n.leaderID = ""
	args := RequestVoteArgs{
		Term:         term,
		CandidateID:  n.id,
		LastLogIndex: n.lastLogIndex,
		LastLogTerm:  n.lastLogTerm,
	}
	peers := make([]Peer, len(n.peers))
	copy(peers, n.peers)
	n.mu.Unlock()

	grants := 1 // our own vote
	for _, p := range peers {
		reply, err := n.transport.RequestVote(ctx, p, args)
		if err != nil {
			continue
		}
		n.mu.Lock()
		if reply.Term > n.term {
			// A newer term exists out there — step down and let it lead.
			n.becomeFollowerLocked(reply.Term)
			n.mu.Unlock()
			return
		}
		if reply.Term == n.term && reply.VoteGranted {
			grants++
			if grants >= n.majority() {
				n.becomeLeaderLocked()
				n.mu.Unlock()
				return
			}
		}
		n.mu.Unlock()
	}

	// Lost this round (no majority) — stay candidate; the next timeout retries.
	n.mu.Lock()
	if n.role == Candidate && grants >= n.majority() {
		n.becomeLeaderLocked()
	}
	n.mu.Unlock()
}

func (n *Node) becomeLeaderLocked() {
	if n.role == Leader {
		return
	}
	n.role = Leader
	n.leaderID = n.id
	log.Printf("raft %s: became leader for term %d", n.id, n.term)
}

// becomeFollowerLocked resets the node to follower with the given term. It also
// resets the election timer so a freshly elected leader gets time to prove
// itself.
func (n *Node) becomeFollowerLocked(term uint64) {
	if n.role == Leader {
		log.Printf("raft %s: stepped down from leader (term %d → %d)", n.id, n.term, term)
	}
	n.role = Follower
	n.term = term
	n.votedFor = ""
	n.leaderID = ""
	n.lastActivity = n.now()
}

// sendHeartbeats pushes an empty AppendEntries to every peer. Replies with a
// higher term step the leader down.
func (n *Node) sendHeartbeats() {
	ctx := context.Background()

	n.mu.Lock()
	args := AppendEntriesArgs{Term: n.term, LeaderID: n.id}
	peers := make([]Peer, len(n.peers))
	copy(peers, n.peers)
	n.mu.Unlock()

	for _, p := range peers {
		go func(p Peer) {
			reply, err := n.transport.AppendEntries(ctx, p, args)
			if err != nil {
				return
			}
			n.mu.Lock()
			defer n.mu.Unlock()
			if reply.Term > n.term {
				n.becomeFollowerLocked(reply.Term)
			}
		}(p)
	}
}

// HandleRequestVote implements the voter side of §5.2.
func (n *Node) HandleRequestVote(args RequestVoteArgs) RequestVoteReply {
	n.mu.Lock()
	defer n.mu.Unlock()

	if args.Term > n.term {
		n.becomeFollowerLocked(args.Term)
	}
	if args.Term < n.term {
		return RequestVoteReply{Term: n.term, VoteGranted: false}
	}

	// args.Term == n.term: grant iff we have not voted yet this term and the
	// candidate's log is at least as up to date as ours.
	if n.votedFor == "" || n.votedFor == args.CandidateID {
		if n.candidateLogUpToDate(args) {
			n.votedFor = args.CandidateID
			n.lastActivity = n.now()
			return RequestVoteReply{Term: n.term, VoteGranted: true}
		}
	}
	return RequestVoteReply{Term: n.term, VoteGranted: false}
}

// candidateLogUpToDate implements §5.4.1. With the empty log of phase 2 every
// candidate qualifies; phase 3 fills in the comparison.
func (n *Node) candidateLogUpToDate(args RequestVoteArgs) bool {
	if args.LastLogTerm != n.lastLogTerm {
		return args.LastLogTerm > n.lastLogTerm
	}
	return args.LastLogIndex >= n.lastLogIndex
}

// HandleAppendEntries implements the follower side of §5.3. Phase 2 only uses
// the heartbeat path (empty entries); log consistency checks arrive with
// replication.
func (n *Node) HandleAppendEntries(args AppendEntriesArgs) AppendEntriesReply {
	if len(args.Entries) > 0 {
		return n.handleAppendEntriesWithLog(args)
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if args.Term < n.term {
		return AppendEntriesReply{Term: n.term, Success: false}
	}
	if args.Term > n.term {
		// A newer term wins: adopt it and clear our vote bookkeeping.
		n.term = args.Term
		n.votedFor = ""
	}
	// Step down from candidate to follower and acknowledge the leader. A
	// same-term heartbeat keeps our vote for that term — re-granting is
	// harmless (the leader already holds a majority) and wiping it could
	// let a term-T challenger collect a second round from us.
	n.role = Follower
	n.leaderID = args.LeaderID
	n.lastActivity = n.now()
	return AppendEntriesReply{Term: n.term, Success: true}
}

// handleAppendEntriesWithLog rejects entries until phase 3 implements the log.
func (n *Node) handleAppendEntriesWithLog(AppendEntriesArgs) AppendEntriesReply {
	return AppendEntriesReply{Term: 0, Success: false}
}
