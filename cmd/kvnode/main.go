// Command kvnode runs a single node of the distributed key-value store.
//
// Phase 2: the node runs the raft loop (Follower/Candidate/Leader), the HTTP
// API is still backed directly by the in-memory store, and /v1/status reports
// the live raft role/term/leader. Log replication and write routing land in the
// next phase.
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
		dataDir = flag.String("data-dir", "", "directory for persistent state (unused until the persistence phase)")
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

	raftNode := raft.NewNode(raftNodeConfig(cfg), raft.NewHTTPTransport())
	raftNode.Start()
	defer raftNode.Stop()

	srv := httpapi.New(directBackend{store: store}, raftStatus{id: cfg.id, peers: len(cfg.peers), node: raftNode})

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
			log.Printf("kvnode data-dir=%s (not used until the persistence phase)", cfg.dataDir)
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

// directBackend adapts the in-memory store to the httpapi.Backend interface.
// Writes always succeed here; the Raft-backed backend (next phase) is where
// ErrUnavailable originates.
type directBackend struct {
	store *kv.Store
}

func (b directBackend) Get(key string) (string, bool) { return b.store.Get(key) }

func (b directBackend) Put(key, value string) error {
	b.store.Put(key, value)
	return nil
}

func (b directBackend) Delete(key string) (bool, error) { return b.store.Delete(key), nil }

// raftStatus reports live raft state through the httpapi.Status shape.
type raftStatus struct {
	id    string
	peers int
	node  *raft.Node
}

func (s raftStatus) Status() httpapi.Status {
	snap := s.node.Snapshot()
	return httpapi.Status{
		ID:     s.id,
		Role:   snap.Role,
		Term:   snap.Term,
		Leader: snap.Leader,
		Peers:  s.peers,
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
