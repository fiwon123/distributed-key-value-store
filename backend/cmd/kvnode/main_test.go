package main

import (
	"errors"
	"testing"

	"github.com/fiwon123/distributed-key-value-store/backend/internal/httpapi"
	"github.com/fiwon123/distributed-key-value-store/backend/internal/kv"
	"github.com/fiwon123/distributed-key-value-store/backend/internal/raft"
)

func TestParsePeers(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    map[string]string
		wantErr bool
	}{
		{name: "empty", in: "", want: map[string]string{}},
		{name: "blank", in: "   ", want: map[string]string{}},
		{
			name: "single",
			in:   "node2=http://localhost:8082",
			want: map[string]string{"node2": "http://localhost:8082"},
		},
		{
			name: "multiple with whitespace",
			in:   " node2=http://localhost:8082 , node3=http://localhost:8083 ",
			want: map[string]string{
				"node2": "http://localhost:8082",
				"node3": "http://localhost:8083",
			},
		},
		{name: "missing url", in: "node2=", wantErr: true},
		{name: "missing separator", in: "node2", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePeers(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePeers(%q) = %v, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePeers(%q) unexpected error: %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("parsePeers(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("parsePeers(%q)[%q] = %q, want %q", tc.in, k, got[k], v)
				}
			}
		})
	}
}

func TestRaftNodeConfigHasTimings(t *testing.T) {
	// Regression: wiring that forgets DefaultConfig leaves election timing at
	// zero, which makes followers re-elect every tick (term churn).
	cfg := raftNodeConfig(runConfig{id: "node1", peers: []raft.Peer{{ID: "node2"}}})
	if cfg.ID != "node1" || len(cfg.Peers) != 1 {
		t.Fatalf("cfg = %+v, want id node1 with 1 peer", cfg)
	}
	if cfg.ElectionTimeoutMin <= 0 || cfg.ElectionTimeoutMax <= cfg.ElectionTimeoutMin {
		t.Fatalf("election timeouts = [%v, %v], want 0 < min < max", cfg.ElectionTimeoutMin, cfg.ElectionTimeoutMax)
	}
	if cfg.HeartbeatInterval <= 0 {
		t.Fatalf("heartbeat interval = %v, want > 0", cfg.HeartbeatInterval)
	}
}

func TestRaftBackendNotLeader(t *testing.T) {
	// A node that is not the leader must reject client operations as
	// unavailable (see httpapi.ErrUnavailable → 503).
	backend := &raftBackend{store: kv.New(), node: raft.NewNode(raft.DefaultConfig(), nil)}
	if _, _, err := backend.Get("k"); !errors.Is(err, httpapi.ErrUnavailable) {
		t.Fatalf("Get error = %v, want ErrUnavailable", err)
	}
	if err := backend.Put("k", "v"); !errors.Is(err, httpapi.ErrUnavailable) {
		t.Fatalf("Put error = %v, want ErrUnavailable", err)
	}
	if _, err := backend.Delete("k"); !errors.Is(err, httpapi.ErrUnavailable) {
		t.Fatalf("Delete error = %v, want ErrUnavailable", err)
	}
}
