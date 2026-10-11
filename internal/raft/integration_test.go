//go:build integration

package raft

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/fiwon123/distributed-key-value-store/internal/kv"
)

// These tests drive the full path — elections, replication, commit, apply —
// across in-process nodes wired by a fake transport, so they need no network.
// Run them with:
//
//	go test -tags=integration ./...
//
// They are excluded from the default `go test ./...` and from `make check`.

type e2eNode struct {
	*Node
	store *kv.Store
}

type e2eCluster struct {
	transport *clusterTransport
	nodes     map[string]*e2eNode
}

func startE2ECluster(t *testing.T, ids ...string) *e2eCluster {
	t.Helper()
	c := &e2eCluster{nodes: make(map[string]*e2eNode, len(ids))}
	order := make([]*Node, 0, len(ids))
	for _, id := range ids {
		var peers []Peer
		for _, other := range ids {
			if other != id {
				peers = append(peers, Peer{ID: other})
			}
		}
		store := kv.New()
		n := NewNode(Config{
			ID:                 id,
			Peers:              peers,
			ElectionTimeoutMin: 25 * time.Millisecond,
			ElectionTimeoutMax: 80 * time.Millisecond,
			HeartbeatInterval:  10 * time.Millisecond,
			ApplyFunc: func(_ uint64, data []byte) error {
				cmd, err := kv.DecodeCommand(data)
				if err != nil {
					return err
				}
				return store.Apply(cmd)
			},
		}, nil)
		c.nodes[id] = &e2eNode{Node: n, store: store}
		order = append(order, n)
	}
	tr := newClusterTransport(order...)
	for _, n := range order {
		n.transport = tr
	}
	c.transport = tr
	t.Cleanup(func() {
		for _, n := range c.nodes {
			n.Stop()
		}
	})
	for _, n := range c.nodes {
		n.Start()
	}
	return c
}

// leader blocks until exactly one live node reports Leader.
func (c *e2eCluster) leader(t *testing.T) *e2eNode {
	t.Helper()
	var found *e2eNode
	waitFor(t, 3*time.Second, func() bool {
		var leaders []*e2eNode
		for _, n := range c.nodes {
			if n.Status().Role == "leader" {
				leaders = append(leaders, n)
			}
		}
		if len(leaders) == 1 {
			found = leaders[0]
			return true
		}
		return false
	})
	return found
}

// kill makes a node unreachable and stops it, as if its process had died.
func (c *e2eCluster) kill(id string) {
	c.transport.remove(id)
	c.nodes[id].Stop()
	delete(c.nodes, id)
}

// e2eMember is a persistent node: it keeps its raft state and KV store on disk.
type e2eMember struct {
	*Node
	store *kv.Store
	dir   string
}

// openPersistentNode builds one durable node with FileStorage and a snapshot
// threshold, applying commands to the returned store.
func openPersistentNode(t *testing.T, id, dir string, peers []Peer, threshold uint64) (*Node, *kv.Store) {
	t.Helper()
	store := kv.New()
	cfg := Config{
		ID:                 id,
		Peers:              peers,
		ElectionTimeoutMin: 25 * time.Millisecond,
		ElectionTimeoutMax: 80 * time.Millisecond,
		HeartbeatInterval:  10 * time.Millisecond,
		Storage:            NewFileStorage(dir),
		SnapshotThreshold:  threshold,
		ApplyFunc: func(_ uint64, data []byte) error {
			cmd, err := kv.DecodeCommand(data)
			if err != nil {
				return err
			}
			return store.Apply(cmd)
		},
		SnapshotFunc: store.Snapshot,
		RestoreFunc:  store.Restore,
	}
	n, err := Open(cfg, nil)
	if err != nil {
		t.Fatalf("Open %s: %v", id, err)
	}
	return n, store
}

func peersFor(ids []string, self string) []Peer {
	var peers []Peer
	for _, id := range ids {
		if id != self {
			peers = append(peers, Peer{ID: id})
		}
	}
	return peers
}

// startPersistentCluster wires Open()-backed nodes with FileStorage under dirs
// and a small snapshot threshold, so the run compacts its log.
func startPersistentCluster(t *testing.T, dirs map[string]string, threshold uint64, ids ...string) (*clusterTransport, map[string]*e2eMember) {
	t.Helper()
	members := make(map[string]*e2eMember, len(ids))
	order := make([]*Node, 0, len(ids))
	for _, id := range ids {
		node, store := openPersistentNode(t, id, dirs[id], peersFor(ids, id), threshold)
		members[id] = &e2eMember{Node: node, store: store, dir: dirs[id]}
		order = append(order, node)
	}
	tr := newClusterTransport(order...)
	for _, n := range order {
		n.transport = tr
	}
	t.Cleanup(func() {
		for _, m := range members {
			m.Stop()
		}
	})
	for _, m := range members {
		m.Start()
	}
	return tr, members
}

// persistentLeader blocks until exactly one member reports Leader.
func persistentLeader(t *testing.T, members map[string]*e2eMember) *e2eMember {
	t.Helper()
	var found *e2eMember
	waitFor(t, 3*time.Second, func() bool {
		var leaders []*e2eMember
		for _, m := range members {
			if m.Status().Role == "leader" {
				leaders = append(leaders, m)
			}
		}
		if len(leaders) == 1 {
			found = leaders[0]
			return true
		}
		return false
	})
	return found
}

func (c *e2eCluster) waitForValue(t *testing.T, key, want string) {
	t.Helper()
	waitFor(t, 2*time.Second, func() bool {
		for _, n := range c.nodes {
			if got, ok := n.store.Get(key); !ok || got != want {
				return false
			}
		}
		return true
	})
}

func propose(t *testing.T, leader *e2eNode, cmd kv.Command) {
	t.Helper()
	data, err := cmd.Encode()
	if err != nil {
		t.Fatalf("encode command: %v", err)
	}
	if err := leader.Propose(data); err != nil {
		t.Fatalf("Propose %+v: %v", cmd, err)
	}
}

func TestLeaderFailureKeepsCommittedValue(t *testing.T) {
	c := startE2ECluster(t, "n1", "n2", "n3")

	leader := c.leader(t)
	propose(t, leader, kv.Command{Op: kv.OpPut, Key: "k", Value: "v1"})
	c.waitForValue(t, "k", "v1")

	killed := leader.id
	c.kill(killed)

	newLeader := c.leader(t)
	if newLeader.id == killed {
		t.Fatalf("dead node %s still reports leader", killed)
	}

	// The committed value survives the leader's failure on every survivor.
	for id, n := range c.nodes {
		if got, ok := n.store.Get("k"); !ok || got != "v1" {
			t.Fatalf("%s: Get(k) = %q, %v after failover; want v1", id, got, ok)
		}
	}

	// And a survivor can still make progress.
	propose(t, newLeader, kv.Command{Op: kv.OpPut, Key: "k", Value: "v2"})
	c.waitForValue(t, "k", "v2")
}

func TestWritesStopWithoutQuorum(t *testing.T) {
	c := startE2ECluster(t, "n1", "n2", "n3")
	leader := c.leader(t)

	// Kill the two followers, leaving the leader with no majority.
	var followers []string
	for id := range c.nodes {
		if id != leader.id {
			followers = append(followers, id)
		}
	}
	for _, id := range followers {
		c.kill(id)
	}

	data, err := kv.Command{Op: kv.OpPut, Key: "k", Value: "v"}.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := leader.propose(data, 400*time.Millisecond); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Propose without quorum = %v, want ErrUnavailable", err)
	}
}

// TestSnapshotsCompactAndSurviveRestart writes enough through a durable
// three-node cluster to trigger snapshots on every node, then reopens a node
// from its data dir and checks the committed state is readable from the
// snapshot — the "restart from snapshot + tail" acceptance criterion.
func TestSnapshotsSurviveRestart(t *testing.T) {
	const threshold = 3
	ids := []string{"n1", "n2", "n3"}
	root := t.TempDir()
	dirs := make(map[string]string, len(ids))
	for _, id := range ids {
		dirs[id] = filepath.Join(root, id)
	}

	tr, members := startPersistentCluster(t, dirs, threshold, ids...)
	leader := persistentLeader(t, members)

	last := ""
	for i := 0; i < int(threshold)+2; i++ {
		last = fmt.Sprintf("k%d", i)
		if err := leader.Propose(mustEncode(t, kv.Command{Op: kv.OpPut, Key: last, Value: "v"})); err != nil {
			t.Fatalf("Propose %s: %v", last, err)
		}
	}

	// Every node applies the writes and compacts its log past the boundary.
	waitFor(t, 3*time.Second, func() bool {
		for _, m := range members {
			m.mu.Lock()
			start, applied := m.logStart, m.lastApplied
			m.mu.Unlock()
			if start == 0 || applied < uint64(threshold)+2 {
				return false
			}
		}
		return true
	})

	// Every node has a durable snapshot on disk.
	for id, m := range members {
		meta, _, err := NewFileStorage(m.dir).LoadSnapshot()
		if err != nil {
			t.Fatalf("%s: LoadSnapshot: %v", id, err)
		}
		if meta.Index == 0 {
			t.Fatalf("%s: no snapshot persisted despite crossing the threshold", id)
		}
	}

	// Restart one follower from its data dir (as a fresh process would) and
	// confirm the committed value is readable from the restored snapshot.
	var victim string
	for id := range members {
		if id != leader.id {
			victim = id
			break
		}
	}
	old := members[victim]
	delete(members, victim) // so cleanup does not stop it twice
	old.Stop()
	tr.remove(victim)

	// The snapshot was taken at the threshold boundary (3), so on restart the
	// state machine reflects everything up to it; the entries beyond are in the
	// log but not yet re-committed.
	node, store := openPersistentNode(t, victim, dirs[victim], peersFor(ids, victim), threshold)
	if got, ok := store.Get("k0"); !ok || got != "v" {
		t.Fatalf("restarted %s: Get(k0) = %q, %v; want v restored from snapshot", victim, got, ok)
	}
	if _, ok := store.Get(last); ok {
		t.Fatalf("restarted %s: Get(%s) present before the log tail is re-committed", victim, last)
	}
	restarted := &e2eMember{Node: node, store: store, dir: dirs[victim]}
	members[victim] = restarted

	// Rejoin and let the leader replicate; the restarted node must accept and
	// commit a new write on top of its recovered state.
	node.transport = tr
	tr.mu.Lock()
	tr.nodes[victim] = node
	tr.mu.Unlock()
	node.Start()

	next := "after-restart"
	if err := leader.Propose(mustEncode(t, kv.Command{Op: kv.OpPut, Key: next, Value: "v2"})); err != nil {
		t.Fatalf("Propose after restart: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool {
		got, ok := store.Get(next)
		return ok && got == "v2"
	})

	// Re-committing also replays the log tail the snapshot did not cover.
	waitFor(t, 3*time.Second, func() bool {
		got, ok := store.Get(last)
		return ok && got == "v"
	})
}

func mustEncode(t *testing.T, cmd kv.Command) []byte {
	t.Helper()
	data, err := cmd.Encode()
	if err != nil {
		t.Fatalf("encode command: %v", err)
	}
	return data
}
