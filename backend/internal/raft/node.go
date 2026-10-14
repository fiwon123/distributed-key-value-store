package raft

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"
)

// Errors returned by Propose and surfaced by callers as 503s.
var (
	// ErrNotLeader means this node cannot accept a write because it is not the
	// current leader.
	ErrNotLeader = errors.New("raft: not the leader")
	// ErrUnavailable means the write was accepted locally but did not commit
	// (lost quorum, lost leadership, or timed out).
	ErrUnavailable = errors.New("raft: write not committed")
)

// Role is a node's Raft role.
type Role int

const (
	// Follower receives leader heartbeats and grants votes.
	Follower Role = iota
	// Candidate is running an election.
	Candidate
	// Leader serves client writes and replicates the log.
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

// Status is a point-in-time view of a node's cluster state, safe to hand to
// status endpoints without holding the node's lock.
type Status struct {
	Role        string
	Term        uint64
	Leader      string
	CommitIndex uint64
	LastApplied uint64
}

// Peer is another node in the cluster.
type Peer struct {
	ID  string
	URL string
}

// Config tunes election/heartbeat timing, identifies the node, and supplies the
// state-machine apply function.
type Config struct {
	ID                 string
	Peers              []Peer // all other nodes, excluding self
	ElectionTimeoutMin time.Duration
	ElectionTimeoutMax time.Duration
	HeartbeatInterval  time.Duration

	// ApplyFunc is called once per committed log entry, in order, with the
	// entry's index and command bytes. It must be deterministic and idempotent.
	ApplyFunc func(index uint64, data []byte) error

	// SnapshotThreshold is the number of entries applied past the last snapshot
	// after which the node snapshots the state machine and compacts the log.
	// 0 disables automatic snapshots.
	SnapshotThreshold uint64

	// SnapshotFunc captures the state machine as bytes at the moment it is
	// called (under the node lock, so the image matches the applied index).
	// When nil, automatic snapshots are disabled.
	SnapshotFunc func() ([]byte, error)

	// RestoreFunc rebuilds the state machine from bytes produced by SnapshotFunc.
	// It is called on startup when a snapshot is loaded, before the log tail is
	// replayed. Required if a snapshot exists on disk.
	RestoreFunc func(data []byte) error

	// Storage persists the log, hard state, and snapshots. When nil, the node
	// keeps all state in memory (used by tests).
	Storage Storage
}

// DefaultConfig returns timings suited to local development and tests:
// 300–600ms election timeout, 100ms heartbeats. Real deployments need larger
// values.
func DefaultConfig() Config {
	return Config{
		ElectionTimeoutMin: 300 * time.Millisecond,
		ElectionTimeoutMax: 600 * time.Millisecond,
		HeartbeatInterval:  100 * time.Millisecond,
		SnapshotThreshold:  1000,
	}
}

// Node is a single raft participant.
//
// Thread safety: the node's state (term, role, votedFor, leader, log,
// commit/apply indexes, timers) is guarded by mu. Network calls happen outside
// the lock — RPC handlers take the lock on the receiving side, and calling out
// while holding our own lock could deadlock two nodes elected in the same term.
type Node struct {
	mu sync.Mutex

	id         string
	peers      []Peer
	cfg        Config
	transport  Transport
	applyFn    func(index uint64, data []byte) error
	snapshotFn func() ([]byte, error)
	restoreFn  func(data []byte) error
	storage    Storage

	role     Role
	term     uint64
	votedFor string
	leaderID string

	// log holds a sentinel entry at logStart followed by contiguous entries, so
	// log[i].Index == logStart + uint64(i). logStart is 0 until the log is
	// compacted, after which it equals the last snapshot's index. Use entryAt to
	// read it.
	log         []LogEntry
	logStart    uint64
	commitIndex uint64
	lastApplied uint64

	// lastIncludedIndex/Term identify the snapshot the log prefix was compacted
	// into; entries at or below lastIncludedIndex are no longer in log.
	lastIncludedIndex uint64
	lastIncludedTerm  uint64

	// snapshotMeta/snapshotData cache the most recent snapshot image so a leader
	// can ship it to a lagging follower (InstallSnapshot) without re-invoking
	// SnapshotFunc. Guards: n.mu.
	snapshotMeta SnapshotMeta
	snapshotData []byte

	// Leader-only replication progress, parallel to peers.
	nextIndex  []uint64
	matchIndex []uint64

	lastActivity time.Time
	now          func() time.Time
	rand         *rand.Rand

	tick  *time.Ticker
	stopC chan struct{}
	wg    sync.WaitGroup
}

// NewNode creates an in-memory node. It does not start running until Start is
// called. Tests and callers that do not need persistence use this; callers that
// persist state use Open.
func NewNode(cfg Config, t Transport) *Node {
	n, err := Open(cfg, t)
	if err != nil {
		// Only reachable when cfg.Storage is set and fails to load. NewNode is
		// for the in-memory case; a load error is surfaced by Open.
		panic(err)
	}
	return n
}

// Open builds a node and, when cfg.Storage is set, recovers its persisted term,
// votedFor, and log. With no storage it behaves like NewNode.
func Open(cfg Config, t Transport) (*Node, error) {
	n := &Node{
		id:           cfg.ID,
		peers:        cfg.Peers,
		cfg:          cfg,
		transport:    t,
		applyFn:      cfg.ApplyFunc,
		snapshotFn:   cfg.SnapshotFunc,
		restoreFn:    cfg.RestoreFunc,
		storage:      cfg.Storage,
		role:         Follower,
		log:          []LogEntry{{}}, // sentinel at index 0
		nextIndex:    make([]uint64, len(cfg.Peers)),
		matchIndex:   make([]uint64, len(cfg.Peers)),
		lastActivity: time.Now(),
		now:          time.Now,
		rand:         rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		stopC:        make(chan struct{}),
	}
	if cfg.Storage == nil {
		return n, nil
	}

	// A snapshot (if any) restores the state machine and moves the log's origin
	// forward; the log tail on disk holds only entries after it.
	meta, data, err := cfg.Storage.LoadSnapshot()
	if err != nil {
		return nil, err
	}
	if meta.Index > 0 {
		if n.restoreFn == nil {
			return nil, fmt.Errorf("raft %s: snapshot at index %d but no RestoreFunc", cfg.ID, meta.Index)
		}
		if err := n.restoreFn(data); err != nil {
			return nil, fmt.Errorf("raft %s: restore snapshot: %w", cfg.ID, err)
		}
		n.lastIncludedIndex = meta.Index
		n.lastIncludedTerm = meta.Term
		n.logStart = meta.Index
		n.log = []LogEntry{{Index: meta.Index, Term: meta.Term}}
		n.commitIndex = meta.Index
		n.lastApplied = meta.Index
		n.snapshotMeta = meta
		n.snapshotData = data
	}

	hard, entries, err := cfg.Storage.Load()
	if err != nil {
		return nil, err
	}
	n.term = hard.Term
	n.votedFor = hard.VotedFor
	for _, entry := range entries {
		// Drop anything the snapshot already covers: a crash between saving the
		// snapshot and truncating the log leaves such entries on disk.
		if entry.Index > n.lastIncludedIndex {
			n.log = append(n.log, entry)
		}
	}
	return n, nil
}

// persistHardStateLocked durably records the current term and vote. Caller must
// hold n.mu.
func (n *Node) persistHardStateLocked() {
	if n.storage == nil {
		return
	}
	if err := n.storage.SaveHardState(HardState{Term: n.term, VotedFor: n.votedFor}); err != nil {
		log.Printf("raft %s: save hard state: %v", n.id, err)
	}
}

// persistEntriesLocked durably appends entries. Caller must hold n.mu.
func (n *Node) persistEntriesLocked(entries []LogEntry) {
	if n.storage == nil || len(entries) == 0 {
		return
	}
	if err := n.storage.AppendEntries(entries); err != nil {
		log.Printf("raft %s: append log entries: %v", n.id, err)
	}
}

// persistTruncateLocked durably removes entries with index >= index. Caller
// must hold n.mu.
func (n *Node) persistTruncateLocked(index uint64) {
	if n.storage == nil {
		return
	}
	if err := n.storage.TruncateFrom(index); err != nil {
		log.Printf("raft %s: truncate log: %v", n.id, err)
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

// Status returns a copy of the node's role/term/leader/commit/apply state.
func (n *Node) Status() Status {
	n.mu.Lock()
	defer n.mu.Unlock()
	return Status{
		Role:        n.role.String(),
		Term:        n.term,
		Leader:      n.leaderID,
		CommitIndex: n.commitIndex,
		LastApplied: n.lastApplied,
	}
}

// Propose appends data to the leader's log and blocks until the entry commits
// and is applied, or until the timeout elapses. It returns ErrNotLeader if this
// node is not the leader, and ErrUnavailable on timeout or lost leadership.
func (n *Node) Propose(data []byte) error {
	return n.propose(data, 3*time.Second)
}

func (n *Node) propose(data []byte, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	n.mu.Lock()
	if n.role != Leader {
		n.mu.Unlock()
		return ErrNotLeader
	}
	index := n.lastLogIndex() + 1
	entry := LogEntry{
		Index: index,
		Term:  n.term,
		Data:  append([]byte(nil), data...),
	}
	n.log = append(n.log, entry)
	n.persistEntriesLocked([]LogEntry{entry})
	n.mu.Unlock()

	// Push the new entry (and any backlog) to peers, then wait for it to apply.
	n.replicateAll()
	if err := n.waitApplied(ctx, index); err != nil {
		return err
	}
	// The entry committed locally; replicate once more so followers learn the
	// new commit index immediately instead of waiting for the next heartbeat.
	n.replicateAll()
	return nil
}

func (n *Node) waitApplied(ctx context.Context, index uint64) error {
	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	for {
		n.mu.Lock()
		applied := n.lastApplied >= index
		leader := n.role == Leader
		n.mu.Unlock()

		if applied {
			return nil
		}
		if !leader {
			return ErrNotLeader
		}
		select {
		case <-ctx.Done():
			return ErrUnavailable
		case <-timer.C:
			timer.Reset(2 * time.Millisecond)
		}
	}
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
		n.replicateAll()
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
	n.persistHardStateLocked()
	args := RequestVoteArgs{
		Term:         term,
		CandidateID:  n.id,
		LastLogIndex: n.lastLogIndex(),
		LastLogTerm:  n.lastLogTerm(),
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
				n.appendLeaderNoopLocked()
				n.mu.Unlock()
				n.replicateAll()
				return
			}
		}
		n.mu.Unlock()
	}

	// Lost this round (no majority) — stay candidate; the next timeout retries.
	n.mu.Lock()
	won := n.role == Candidate && grants >= n.majority()
	if won {
		n.becomeLeaderLocked()
		n.appendLeaderNoopLocked()
	}
	n.mu.Unlock()
	if won {
		n.replicateAll()
	}
}

func (n *Node) becomeLeaderLocked() {
	if n.role == Leader {
		return
	}
	n.role = Leader
	n.leaderID = n.id
	next := n.lastLogIndex() + 1
	for i := range n.nextIndex {
		n.nextIndex[i] = next
		n.matchIndex[i] = 0
	}
	log.Printf("raft %s: became leader for term %d", n.id, n.term)
}

// appendLeaderNoopLocked appends an empty entry for the current term and points
// replication at it. A leader commits an entry only once one from its own term
// is stored on a majority (§5.4.2), so without a current-term entry, entries
// committed by a previous term stay uncommitted — and unapplied — forever after
// a restart. Committing the no-op advances commitIndex past those entries.
//
// The election routine calls this right after becomeLeaderLocked, the only path
// to leadership in production. Caller must hold n.mu and be Leader.
func (n *Node) appendLeaderNoopLocked() {
	index := n.lastLogIndex() + 1
	noop := LogEntry{Index: index, Term: n.term}
	n.log = append(n.log, noop)
	n.persistEntriesLocked([]LogEntry{noop})
	// Point every peer at the no-op so it is replicated immediately; on a
	// mismatch nextIndex backs off as usual.
	for i := range n.nextIndex {
		n.nextIndex[i] = index
		n.matchIndex[i] = 0
	}
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
	n.persistHardStateLocked()
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
	// candidate's log is at least as up to date as ours (§5.4.1).
	if n.votedFor == "" || n.votedFor == args.CandidateID {
		if n.candidateLogUpToDateLocked(args) {
			n.votedFor = args.CandidateID
			n.lastActivity = n.now()
			n.persistHardStateLocked()
			return RequestVoteReply{Term: n.term, VoteGranted: true}
		}
	}
	return RequestVoteReply{Term: n.term, VoteGranted: false}
}

// candidateLogUpToDateLocked implements the up-to-date check of §5.4.1.
func (n *Node) candidateLogUpToDateLocked(args RequestVoteArgs) bool {
	lastIndex, lastTerm := n.lastLogIndex(), n.lastLogTerm()
	if args.LastLogTerm != lastTerm {
		return args.LastLogTerm > lastTerm
	}
	return args.LastLogIndex >= lastIndex
}
