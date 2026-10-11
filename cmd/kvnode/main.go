// Command kvnode runs a single node of the distributed key-value store.
//
// Phase 1 is a single-node server: the HTTP API is backed directly by the
// in-memory store. Later phases add Raft (elections, replication), persistence,
// and a peer-to-peer transport behind the same HTTP surface.
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
		peers:   peers,
		dataDir: *dataDir,
	}); err != nil {
		log.Fatalf("kvnode: %v", err)
	}
}

type runConfig struct {
	id      string
	httpAdr string
	peers   map[string]string
	dataDir string
}

func run(cfg runConfig) error {
	store := kv.New()
	srv := httpapi.New(directBackend{store: store}, singleStatus{id: cfg.id, peers: len(cfg.peers)})

	httpServer := &http.Server{
		Addr:              cfg.httpAdr,
		Handler:           srv.Handler(),
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

// directBackend adapts the in-memory store to the httpapi.Backend interface.
// Writes always succeed here; the Raft-backed backend (later phase) is where
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

// singleStatus reports a single-node status until Raft is wired in.
type singleStatus struct {
	id    string
	peers int
}

func (s singleStatus) Status() httpapi.Status {
	return httpapi.Status{
		ID:     s.id,
		Role:   "single",
		Leader: s.id,
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
