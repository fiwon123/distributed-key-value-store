package raft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Transport delivers raft RPCs to peers. The HTTP implementation talks to the
// peer's POST /raft/* endpoints; tests use fakes so no real peer is ever
// contacted from unit tests.
type Transport interface {
	RequestVote(ctx context.Context, peer Peer, args RequestVoteArgs) (RequestVoteReply, error)
	AppendEntries(ctx context.Context, peer Peer, args AppendEntriesArgs) (AppendEntriesReply, error)
	InstallSnapshot(ctx context.Context, peer Peer, args InstallSnapshotArgs) (InstallSnapshotReply, error)
}

// HTTPTransport sends raft RPCs over HTTP with JSON bodies.
type HTTPTransport struct {
	client  *http.Client
	timeout time.Duration
}

// NewHTTPTransport returns a transport that dials peers over HTTP.
func NewHTTPTransport() *HTTPTransport {
	return &HTTPTransport{
		client:  &http.Client{},
		timeout: 2 * time.Second,
	}
}

// RequestVote POSTs a vote request to the peer.
func (t *HTTPTransport) RequestVote(ctx context.Context, peer Peer, args RequestVoteArgs) (RequestVoteReply, error) {
	var reply RequestVoteReply
	err := t.post(ctx, peer, "/raft/request_vote", args, &reply)
	return reply, err
}

// AppendEntries POSTs an AppendEntries RPC (heartbeat or log entries) to the peer.
func (t *HTTPTransport) AppendEntries(ctx context.Context, peer Peer, args AppendEntriesArgs) (AppendEntriesReply, error) {
	var reply AppendEntriesReply
	err := t.post(ctx, peer, "/raft/append_entries", args, &reply)
	return reply, err
}

// InstallSnapshot POSTs a full state-machine snapshot to a lagging peer.
func (t *HTTPTransport) InstallSnapshot(ctx context.Context, peer Peer, args InstallSnapshotArgs) (InstallSnapshotReply, error) {
	var reply InstallSnapshotReply
	err := t.post(ctx, peer, "/raft/install_snapshot", args, &reply)
	return reply, err
}

func (t *HTTPTransport) post(ctx context.Context, peer Peer, path string, reqBody, respBody any) error {
	if peer.URL == "" {
		return fmt.Errorf("raft: peer %q has no URL", peer.ID)
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("raft: marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, peer.URL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("raft: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("raft: post to %s: %w", peer.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("raft: peer %s returned status %d", peer.ID, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return fmt.Errorf("raft: decode reply from %s: %w", peer.ID, err)
	}
	return nil
}

// NewHTTPHandler exposes a node's raft RPC endpoints over HTTP. Mount it on the
// node's server under /raft/.
func NewHTTPHandler(n *Node) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /raft/request_vote", func(w http.ResponseWriter, r *http.Request) {
		var args RequestVoteArgs
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		writeJSON(w, n.HandleRequestVote(args))
	})
	mux.HandleFunc("POST /raft/append_entries", func(w http.ResponseWriter, r *http.Request) {
		var args AppendEntriesArgs
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		writeJSON(w, n.HandleAppendEntries(args))
	})
	mux.HandleFunc("POST /raft/install_snapshot", func(w http.ResponseWriter, r *http.Request) {
		var args InstallSnapshotArgs
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		writeJSON(w, n.HandleInstallSnapshot(args))
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
