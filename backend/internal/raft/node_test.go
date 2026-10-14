package raft

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

const (
	testElectionMin = 20 * time.Millisecond
	testElectionMax = 60 * time.Millisecond
	testHeartbeat   = 10 * time.Millisecond
)

func testConfig(id string, peers []Peer) Config {
	return Config{
		ID:                 id,
		Peers:              peers,
		ElectionTimeoutMin: testElectionMin,
		ElectionTimeoutMax: testElectionMax,
		HeartbeatInterval:  testHeartbeat,
	}
}

// stubTransport lets tests script RPC outcomes deterministically.
type stubTransport struct {
	mu    sync.Mutex
	vote  func(peer Peer, args RequestVoteArgs) (RequestVoteReply, error)
	appnd func(peer Peer, args AppendEntriesArgs) (AppendEntriesReply, error)
	snap  func(peer Peer, args InstallSnapshotArgs) (InstallSnapshotReply, error)
}

func (s *stubTransport) RequestVote(_ context.Context, peer Peer, args RequestVoteArgs) (RequestVoteReply, error) {
	if s.vote == nil {
		return RequestVoteReply{}, errors.New("unexpected RequestVote")
	}
	return s.vote(peer, args)
}

func (s *stubTransport) AppendEntries(_ context.Context, peer Peer, args AppendEntriesArgs) (AppendEntriesReply, error) {
	if s.appnd == nil {
		return AppendEntriesReply{}, errors.New("unexpected AppendEntries")
	}
	return s.appnd(peer, args)
}

func (s *stubTransport) InstallSnapshot(_ context.Context, peer Peer, args InstallSnapshotArgs) (InstallSnapshotReply, error) {
	if s.snap == nil {
		return InstallSnapshotReply{}, errors.New("unexpected InstallSnapshot")
	}
	return s.snap(peer, args)
}

// clusterTransport routes RPCs to in-process nodes by peer id — a fake, so
// unit tests never touch the network.
type clusterTransport struct {
	mu    sync.Mutex
	nodes map[string]*Node
}

func newClusterTransport(nodes ...*Node) *clusterTransport {
	t := &clusterTransport{nodes: make(map[string]*Node)}
	for _, n := range nodes {
		t.nodes[n.id] = n
	}
	return t
}

func (c *clusterTransport) node(id string) *Node {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nodes[id]
}

func (c *clusterTransport) RequestVote(_ context.Context, peer Peer, args RequestVoteArgs) (RequestVoteReply, error) {
	n := c.node(peer.ID)
	if n == nil {
		return RequestVoteReply{}, errors.New("no such node")
	}
	return n.HandleRequestVote(args), nil
}

func (c *clusterTransport) AppendEntries(_ context.Context, peer Peer, args AppendEntriesArgs) (AppendEntriesReply, error) {
	n := c.node(peer.ID)
	if n == nil {
		return AppendEntriesReply{}, errors.New("no such node")
	}
	return n.HandleAppendEntries(args), nil
}

func (c *clusterTransport) InstallSnapshot(_ context.Context, peer Peer, args InstallSnapshotArgs) (InstallSnapshotReply, error) {
	n := c.node(peer.ID)
	if n == nil {
		return InstallSnapshotReply{}, errors.New("no such node")
	}
	return n.HandleInstallSnapshot(args), nil
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within", d)
}

func TestRequestVoteGrantedFreshTerm(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})
	reply := n.HandleRequestVote(RequestVoteArgs{Term: 1, CandidateID: "node2"})
	if !reply.VoteGranted {
		t.Fatal("vote should be granted: no vote cast yet this term")
	}
	if term := n.Status().Term; term != 1 {
		t.Fatalf("term = %d, want 1", term)
	}
}

func TestRequestVoteDeniesStaleTerm(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})
	n.HandleRequestVote(RequestVoteArgs{Term: 2, CandidateID: "nodeA"})
	reply := n.HandleRequestVote(RequestVoteArgs{Term: 1, CandidateID: "nodeB"})
	if reply.VoteGranted || reply.Term != 2 {
		t.Fatalf("stale-term request: reply = %+v, want denied with term 2", reply)
	}
}

func TestRequestVoteAtMostOnePerTerm(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})
	if r := n.HandleRequestVote(RequestVoteArgs{Term: 1, CandidateID: "nodeA"}); !r.VoteGranted {
		t.Fatal("first vote in term 1 should be granted")
	}
	if r := n.HandleRequestVote(RequestVoteArgs{Term: 1, CandidateID: "nodeB"}); r.VoteGranted {
		t.Fatal("second vote in same term must be denied")
	}
	// A later term unlocks voting again.
	if r := n.HandleRequestVote(RequestVoteArgs{Term: 2, CandidateID: "nodeB"}); !r.VoteGranted {
		t.Fatal("vote in a new term should be granted")
	}
}

func TestRequestVoteStepsDownOnHigherTerm(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})
	// node1 has already voted for itself in term 3 (e.g. it was a candidate).
	n.mu.Lock()
	n.term = 3
	n.votedFor = "node1"
	n.role = Candidate
	n.mu.Unlock()

	reply := n.HandleRequestVote(RequestVoteArgs{Term: 4, CandidateID: "node9"})
	if !reply.VoteGranted {
		t.Fatal("higher-term candidate must be granted the vote")
	}
	snap := n.Status()
	if snap.Role != "follower" || snap.Term != 4 {
		t.Fatalf("snapshot = %+v, want follower term 4", snap)
	}
}

func TestRunElectionWinsWithMajority(t *testing.T) {
	peers := []Peer{{ID: "node2"}, {ID: "node3"}}
	stub := &stubTransport{
		vote: func(_ Peer, args RequestVoteArgs) (RequestVoteReply, error) {
			return RequestVoteReply{Term: args.Term, VoteGranted: true}, nil
		},
	}
	n := NewNode(testConfig("node1", peers), stub)

	n.runElection()

	snap := n.Status()
	if snap.Role != "leader" {
		t.Fatalf("role = %s, want leader", snap.Role)
	}
	if snap.Leader != "node1" {
		t.Fatalf("leader = %s, want node1", snap.Leader)
	}
	if snap.Term != 1 {
		t.Fatalf("term = %d, want 1", snap.Term)
	}
}

func TestElectionAppendsNoopEntry(t *testing.T) {
	peers := []Peer{{ID: "node2"}, {ID: "node3"}}
	stub := &stubTransport{
		vote: func(_ Peer, args RequestVoteArgs) (RequestVoteReply, error) {
			return RequestVoteReply{Term: args.Term, VoteGranted: true}, nil
		},
		appnd: func(_ Peer, _ AppendEntriesArgs) (AppendEntriesReply, error) {
			return AppendEntriesReply{Term: 1, Success: true}, nil
		},
	}
	n := NewNode(testConfig("node1", peers), stub)

	n.runElection()

	if snap := n.Status(); snap.Role != "leader" {
		t.Fatalf("role = %s, want leader", snap.Role)
	}
	n.mu.Lock()
	last := n.log[len(n.log)-1]
	n.mu.Unlock()
	if last.Term != n.Status().Term {
		t.Fatalf("last entry term = %d, want current term %d", last.Term, n.Status().Term)
	}
	if len(last.Data) != 0 {
		t.Fatalf("last entry data = %q, want an empty no-op", last.Data)
	}
}

func TestRestartAppliesCommittedEntriesWithoutNewWrite(t *testing.T) {
	dir := t.TempDir()
	state := &memState{}
	cfg := testConfig("n1", nil)
	cfg.Storage = NewFileStorage(dir)
	cfg.SnapshotThreshold = 0 // keep the whole log; snapshots are covered elsewhere
	cfg.ApplyFunc = state.apply

	n, err := Open(cfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	n.Start()
	waitFor(t, 2*time.Second, func() bool { return n.Status().Role == "leader" })
	for _, v := range []string{"a", "b"} {
		if err := n.Propose([]byte(v)); err != nil {
			t.Fatalf("Propose %q: %v", v, err)
		}
	}
	n.Stop()

	// A fresh process on the same data dir recovers commitIndex 0 and a purely
	// prior-term log. Electing itself appends a current-term no-op, which must
	// commit and apply the recovered entries without any new client write.
	fresh := &memState{}
	restartCfg := testConfig("n1", nil)
	restartCfg.Storage = NewFileStorage(dir)
	restartCfg.SnapshotThreshold = 0
	restartCfg.ApplyFunc = fresh.apply
	n2, err := Open(restartCfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open (restart): %v", err)
	}
	n2.Start()
	defer n2.Stop()

	waitFor(t, 2*time.Second, func() bool {
		return reflect.DeepEqual(fresh.contents(), []string{"a", "b"})
	})
}

func TestRunElectionLosesWithoutMajority(t *testing.T) {
	peers := []Peer{{ID: "node2"}, {ID: "node3"}}
	stub := &stubTransport{
		vote: func(_ Peer, args RequestVoteArgs) (RequestVoteReply, error) {
			return RequestVoteReply{Term: args.Term, VoteGranted: false}, nil
		},
	}
	n := NewNode(testConfig("node1", peers), stub)

	n.runElection()

	snap := n.Status()
	if snap.Role != "candidate" {
		t.Fatalf("role = %s, want candidate (lost the round)", snap.Role)
	}
	if snap.Term != 1 {
		t.Fatalf("term = %d, want 1 (term must still advance on a lost round)", snap.Term)
	}
}

func TestElectionStepsDownOnHigherTermReply(t *testing.T) {
	peers := []Peer{{ID: "node2"}}
	stub := &stubTransport{
		vote: func(_ Peer, _ RequestVoteArgs) (RequestVoteReply, error) {
			return RequestVoteReply{Term: 9, VoteGranted: false}, nil
		},
	}
	n := NewNode(testConfig("node1", peers), stub)

	n.runElection()

	snap := n.Status()
	if snap.Role != "follower" {
		t.Fatalf("role = %s, want follower after seeing term 9", snap.Role)
	}
	if snap.Term != 9 {
		t.Fatalf("term = %d, want 9", snap.Term)
	}
}

func TestAppendEntriesHeartbeatResetsTimer(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})

	// Age the node past its election timeout.
	n.mu.Lock()
	n.lastActivity = n.now().Add(-time.Second)
	n.mu.Unlock()

	reply := n.HandleAppendEntries(AppendEntriesArgs{Term: 2, LeaderID: "node2"})
	if !reply.Success {
		t.Fatal("heartbeat with higher term should succeed")
	}
	snap := n.Status()
	if snap.Role != "follower" || snap.Leader != "node2" || snap.Term != 2 {
		t.Fatalf("snapshot = %+v, want follower, leader node2, term 2", snap)
	}

	n.mu.Lock()
	age := n.now().Sub(n.lastActivity)
	n.mu.Unlock()
	if age > testElectionMin {
		t.Fatalf("activity age = %v, want reset (<= %v)", age, testElectionMin)
	}
}

func TestAppendEntriesDeniesStaleTerm(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})
	n.HandleAppendEntries(AppendEntriesArgs{Term: 2, LeaderID: "node2"})

	reply := n.HandleAppendEntries(AppendEntriesArgs{Term: 1, LeaderID: "node3"})
	if reply.Success || reply.Term != 2 {
		t.Fatalf("stale heartbeat: reply = %+v, want denied with term 2", reply)
	}
}

func TestLeaderStepsDownOnHigherTermHeartbeatReply(t *testing.T) {
	peers := []Peer{{ID: "node2"}}
	stub := &stubTransport{
		appnd: func(_ Peer, _ AppendEntriesArgs) (AppendEntriesReply, error) {
			return AppendEntriesReply{Term: 7, Success: false}, nil
		},
	}
	n := NewNode(testConfig("node1", peers), stub)
	n.mu.Lock()
	n.role = Leader
	n.term = 4
	n.leaderID = "node1"
	n.mu.Unlock()

	n.replicateAll()

	waitFor(t, time.Second, func() bool {
		snap := n.Status()
		return snap.Role == "follower" && snap.Term == 7
	})
}

func TestSingleNodeBecomesLeader(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})
	n.Start()
	defer n.Stop()

	waitFor(t, 2*time.Second, func() bool {
		return n.Status().Role == "leader"
	})
	snap := n.Status()
	if snap.Leader != "node1" {
		t.Fatalf("leader = %s, want node1 (quorum of one)", snap.Leader)
	}
}

func TestThreeNodeClusterElectsLeader(t *testing.T) {
	nodeIDs := []string{"node1", "node2", "node3"}
	var nodes []*Node
	for _, id := range nodeIDs {
		var peers []Peer
		for _, other := range nodeIDs {
			if other != id {
				peers = append(peers, Peer{ID: other})
			}
		}
		nodes = append(nodes, NewNode(testConfig(id, peers), nil))
	}
	transport := newClusterTransport(nodes...)
	for _, n := range nodes {
		n.transport = transport
	}

	for _, n := range nodes {
		n.Start()
	}
	defer func() {
		for _, n := range nodes {
			n.Stop()
		}
	}()

	// Eventually exactly one node is leader and every follower knows it.
	waitFor(t, 5*time.Second, func() bool {
		var leaders []string
		snaps := make(map[string]Status)
		for _, n := range nodes {
			snaps[n.id] = n.Status()
			if snaps[n.id].Role == "leader" {
				leaders = append(leaders, n.id)
			}
		}
		if len(leaders) != 1 {
			return false
		}
		for id, snap := range snaps {
			if id != leaders[0] && snap.Leader != leaders[0] {
				return false
			}
		}
		return true
	})
}
