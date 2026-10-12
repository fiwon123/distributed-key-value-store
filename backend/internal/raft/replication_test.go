package raft

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// recorder captures apply callbacks per node id.
type recorder struct {
	mu   sync.Mutex
	seen map[string][]uint64
}

func newRecorder() *recorder { return &recorder{seen: make(map[string][]uint64)} }

func (r *recorder) fn(id string) func(uint64, []byte) error {
	return func(index uint64, _ []byte) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.seen[id] = append(r.seen[id], index)
		return nil
	}
}

func (r *recorder) count(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen[id])
}

// newTestCluster wires n in-process nodes to each other with a fake transport.
// Nodes are not started, so no elections run — a test drives replication by
// making one node the leader with forceLeader. The returned transport lets a
// test simulate a downed peer by removing it.
func newTestCluster(t *testing.T, ids ...string) (*clusterTransport, map[string]*Node, *recorder) {
	t.Helper()
	rec := newRecorder()
	nodes := make(map[string]*Node, len(ids))
	list := make([]*Node, 0, len(ids))
	for _, id := range ids {
		var peers []Peer
		for _, other := range ids {
			if other != id {
				peers = append(peers, Peer{ID: other})
			}
		}
		cfg := testConfig(id, peers)
		cfg.ApplyFunc = rec.fn(id)
		n := NewNode(cfg, nil)
		nodes[id] = n
		list = append(list, n)
	}
	tr := newClusterTransport(list...)
	for _, n := range list {
		n.transport = tr
	}
	return tr, nodes, rec
}

// forceLeader makes n a leader of term 1 and initialises its replication
// state, without running elections. Caller must not have started n.
func forceLeader(n *Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.term = 1
	n.votedFor = n.id
	n.role = Follower
	n.becomeLeaderLocked()
	n.persistHardStateLocked()
}

// remove makes a peer unreachable, as if its process had died.
func (c *clusterTransport) remove(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.nodes, id)
}

// --- follower side of AppendEntries -----------------------------------------

func TestHandleAppendEntriesCommitsAndApplies(t *testing.T) {
	var applied []uint64
	cfg := testConfig("n1", nil)
	cfg.ApplyFunc = func(index uint64, _ []byte) error {
		applied = append(applied, index)
		return nil
	}
	n := NewNode(cfg, &stubTransport{})

	reply := n.HandleAppendEntries(AppendEntriesArgs{
		Term:         1,
		LeaderID:     "n2",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      []LogEntry{{Index: 1, Term: 1, Data: []byte("a")}},
		LeaderCommit: 1,
	})
	if !reply.Success {
		t.Fatalf("reply = %+v, want success", reply)
	}
	snap := n.Status()
	if snap.CommitIndex != 1 || snap.LastApplied != 1 {
		t.Fatalf("commit=%d applied=%d, want 1/1", snap.CommitIndex, snap.LastApplied)
	}
	if len(applied) != 1 || applied[0] != 1 {
		t.Fatalf("applied indices = %v, want [1]", applied)
	}
}

func TestHandleAppendEntriesRejectsMismatchedPrevTerm(t *testing.T) {
	n := NewNode(testConfig("n1", nil), &stubTransport{})
	// Seed the follower's log with term 1 at index 1.
	n.HandleAppendEntries(AppendEntriesArgs{
		Term: 1, LeaderID: "n2",
		Entries: []LogEntry{{Index: 1, Term: 1, Data: []byte("a")}},
	})

	// Leader claims index 1 was written in term 2 — inconsistent.
	reply := n.HandleAppendEntries(AppendEntriesArgs{
		Term: 2, LeaderID: "n2",
		PrevLogIndex: 1, PrevLogTerm: 2,
		Entries: []LogEntry{{Index: 2, Term: 2, Data: []byte("b")}},
	})
	if reply.Success {
		t.Fatalf("reply = %+v, want rejection", reply)
	}
}

func TestHandleAppendEntriesTruncatesConflictingTail(t *testing.T) {
	n := NewNode(testConfig("n1", nil), &stubTransport{})
	// Follower has an uncommitted entry in term 1.
	n.HandleAppendEntries(AppendEntriesArgs{
		Term: 1, LeaderID: "n2",
		Entries: []LogEntry{
			{Index: 1, Term: 1, Data: []byte("a")},
			{Index: 2, Term: 1, Data: []byte("stale")},
		},
	})

	// A new leader in term 2 overwrites index 2 with a different entry.
	reply := n.HandleAppendEntries(AppendEntriesArgs{
		Term: 2, LeaderID: "n3",
		PrevLogIndex: 1, PrevLogTerm: 1,
		Entries: []LogEntry{{Index: 2, Term: 2, Data: []byte("fresh")}},
	})
	if !reply.Success {
		t.Fatalf("reply = %+v, want success", reply)
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.log) != 3 {
		t.Fatalf("log length = %d, want 3 (sentinel + 2)", len(n.log))
	}
	if n.log[2].Term != 2 {
		t.Fatalf("index 2 term = %d, want 2 after truncation", n.log[2].Term)
	}
}

// --- replicated log across a cluster ----------------------------------------

func TestProposeReplicatesAndCommits(t *testing.T) {
	_, nodes, rec := newTestCluster(t, "n1", "n2", "n3")
	leader := nodes["n1"]
	forceLeader(leader)

	if err := leader.Propose([]byte("hello")); err != nil {
		t.Fatalf("Propose: %v", err)
	}

	for id := range nodes {
		if got := rec.count(id); got != 1 {
			t.Fatalf("%s applied %d entries, want 1", id, got)
		}
	}
	for id, n := range nodes {
		snap := n.Status()
		if snap.CommitIndex != 1 || snap.LastApplied != 1 {
			t.Fatalf("%s: commit=%d applied=%d, want 1/1", id, snap.CommitIndex, snap.LastApplied)
		}
	}
}

func TestProposeWithOnePeerDownStillCommits(t *testing.T) {
	tr, nodes, rec := newTestCluster(t, "n1", "n2", "n3")
	leader := nodes["n1"]
	forceLeader(leader)

	tr.remove("n2") // simulate the peer's process dying

	if err := leader.Propose([]byte("x")); err != nil {
		t.Fatalf("Propose with one peer down: %v", err)
	}
	if got := rec.count("n1"); got != 1 {
		t.Fatalf("leader applied %d entries, want 1", got)
	}
	if got := rec.count("n3"); got != 1 {
		t.Fatalf("surviving follower applied %d entries, want 1", got)
	}
}

func TestProposeWithoutQuorumFails(t *testing.T) {
	tr, nodes, _ := newTestCluster(t, "n1", "n2", "n3")
	leader := nodes["n1"]
	forceLeader(leader)

	tr.remove("n2")
	tr.remove("n3") // only the leader remains reachable

	err := leader.propose([]byte("x"), 200*time.Millisecond)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Propose error = %v, want ErrUnavailable", err)
	}
}

func TestFollowerProposeReturnsNotLeader(t *testing.T) {
	_, nodes, _ := newTestCluster(t, "n1", "n2", "n3")
	forceLeader(nodes["n1"])

	err := nodes["n2"].Propose([]byte("x"))
	if !errors.Is(err, ErrNotLeader) {
		t.Fatalf("Propose error = %v, want ErrNotLeader", err)
	}
}

func TestSingleNodeClusterCommits(t *testing.T) {
	n := NewNode(testConfig("n1", nil), &stubTransport{})
	forceLeader(n)

	if err := n.Propose([]byte("x")); err != nil {
		t.Fatalf("Propose on single-node cluster: %v", err)
	}
	snap := n.Status()
	if snap.CommitIndex != 1 || snap.LastApplied != 1 {
		t.Fatalf("commit=%d applied=%d, want 1/1", snap.CommitIndex, snap.LastApplied)
	}
}
