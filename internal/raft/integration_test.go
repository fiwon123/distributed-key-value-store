//go:build integration

package raft

import (
	"errors"
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
			if n.Snapshot().Role == "leader" {
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
