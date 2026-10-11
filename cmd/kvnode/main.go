// Command kvnode runs a single node of the distributed key-value store.
//
// The node runs the raft loop (Follower/Candidate/Leader log replication),
// backs writes with the replicated log, and serves /v1. When -data-dir is set,
// the current term, vote, and log entries are persisted and recovered on
// restart.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fiwon123/distributed-key-value-store/internal/httpapi"
	"github.com/fiwon123/distributed-key-value-store/internal/kv"
	"github.com/fiwon123/distributed-key-value-store/internal/raft"
)

func main() {
	var (
		id      = flag.String("id", "node1", "unique node id")
		httpAdr = flag.String("http", ":8080", "HTTP listen address")
		peersFl = flag.String("peers", "", "comma-separated peers as id=url (e.g. node2=http://localhost:8082)")
		dataDir = flag.String("data-dir", "", "directory for persistent raft state (empty = in-memory)")
	)
	flag.Parse()

	peers, err := parsePeers(*peersFl)
	if err != nil {
		log.Fatalf("kvnode: %v", err)
	}

	if err := run(runConfig{
		id:      *id,
		httpAdr: *httpAdr,
		peers:   peersFromMap(peers),
		dataDir: *dataDir,
	}); err != nil {
		log.Fatalf("kvnode: %v", err)
	}
}

type runConfig struct {
	id      string
	httpAdr string
	peers   []raft.Peer
	dataDir string
}

func run(cfg runConfig) error {
	store := kv.New()

	raftCfg := raftNodeConfig(cfg)
	raftCfg.ApplyFunc = func(_ uint64, data []byte) error {
		cmd, err := kv.DecodeCommand(data)
		if err != nil {
			return err
		}
		return store.Apply(cmd)
	}
	if cfg.dataDir != "" {
		raftCfg.Storage = raft.NewFileStorage(cfg.dataDir)
	}
	raftNode, err := raft.Open(raftCfg, raft.NewHTTPTransport())
	if err != nil {
		return err
	}
	raftNode.Start()
	defer raftNode.Stop()

	backend := &raftBackend{store: store, node: raftNode}
	srv := httpapi.New(backend, raftStatus{id: cfg.id, peers: len(cfg.peers), node: raftNode})

	root := http.NewServeMux()
	root.Handle("/raft/", raft.NewHTTPHandler(raftNode))
	root.Handle("/", srv.Handler())

	httpServer := &http.Server{
		Addr:              cfg.httpAdr,
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("kvnode id=%s listening on %s (peers=%d)", cfg.id, cfg.httpAdr, len(cfg.peers))
		if cfg.dataDir != "" {
			log.Printf("kvnode persisting raft state to %s", cfg.dataDir)
		}
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Print("kvnode shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

// raftNodeConfig returns a raft config with default election/heartbeat timing
// and this node's identity and peers.
func raftNodeConfig(cfg runConfig) raft.Config {
	c := raft.DefaultConfig()
	c.ID = cfg.id
	c.Peers = cfg.peers
	return c
}

// raftBackend drives writes through the raft log and serves reads from the
// applied in-memory state. Reads and writes are leader-only for the MVP; a
// non-leader returns httpapi.ErrUnavailable (→ 503).
type raftBackend struct {
	store *kv.Store
	node  *raft.Node
}

func (b *raftBackend) Get(key string) (string, bool, error) {
	if b.node.Snapshot().Role != "leader" {
		return "", false, httpapi.ErrUnavailable
	}
	value, ok := b.store.Get(key)
	return value, ok, nil
}

func (b *raftBackend) Put(key, value string) error {
	data, err := kv.Command{Op: kv.OpPut, Key: key, Value: value}.Encode()
	if err != nil {
		return err
	}
	return b.propose(data)
}

func (b *raftBackend) Delete(key string) (bool, error) {
	data, err := kv.Command{Op: kv.OpDelete, Key: key}.Encode()
	if err != nil {
		return false, err
	}
	if err := b.propose(data); err != nil {
		return false, err
	}
	return true, nil
}

// propose submits an encoded command to raft and maps raft errors to the API's
// ErrUnavailable so the handler returns 503 (with the leader hint).
func (b *raftBackend) propose(data []byte) error {
	err := b.node.Propose(data)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, raft.ErrNotLeader), errors.Is(err, raft.ErrUnavailable):
		return httpapi.ErrUnavailable
	default:
		return err
	}
}

// raftStatus reports live raft state through the httpapi.Status shape.
type raftStatus struct {
	id    string
	peers int
	node  *raft.Node
}

func (s raftStatus) Status() httpapi.Status {
	snap := s.node.Snapshot()
	return httpapi.Status{
		ID:          s.id,
		Role:        snap.Role,
		Term:        snap.Term,
		Leader:      snap.Leader,
		CommitIndex: snap.CommitIndex,
		LastApplied: snap.LastApplied,
		Peers:       s.peers,
	}
}

// parsePeers parses a comma-separated list of id=url entries into a map. An
// empty string yields an empty (non-nil) map.
func parsePeers(s string) (map[string]string, error) {
	peers := make(map[string]string)
	if strings.TrimSpace(s) == "" {
		return peers, nil
	}
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		id, url, ok := strings.Cut(entry, "=")
		id = strings.TrimSpace(id)
		url = strings.TrimSpace(url)
		if !ok || id == "" || url == "" {
			return nil, fmt.Errorf("invalid peer %q: want id=url", entry)
		}
		peers[id] = url
	}
	return peers, nil
}

// peersFromMap converts a parsed peer map to a slice of raft.Peer. Map
// iteration order is random, which is fine — raft peers are addressed by ID.
func peersFromMap(m map[string]string) []raft.Peer {
	out := make([]raft.Peer, 0, len(m))
	for id, url := range m {
		out = append(out, raft.Peer{ID: id, URL: url})
	}
	return out
}
