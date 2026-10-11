package raft

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPHandlerRequestVoteRoundTrip(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})
	srv := httptest.NewServer(NewHTTPHandler(n))
	defer srv.Close()

	body, err := json.Marshal(RequestVoteArgs{Term: 3, CandidateID: "node2"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(srv.URL+"/raft/request_vote", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var reply RequestVoteReply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if !reply.VoteGranted || reply.Term != 3 {
		t.Fatalf("reply = %+v, want granted term 3", reply)
	}
}

func TestHTTPHandlerAppendEntriesRoundTrip(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})
	srv := httptest.NewServer(NewHTTPHandler(n))
	defer srv.Close()

	body, err := json.Marshal(AppendEntriesArgs{Term: 3, LeaderID: "node2"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(srv.URL+"/raft/append_entries", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	var reply AppendEntriesReply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if !reply.Success || reply.Term != 3 {
		t.Fatalf("reply = %+v, want success term 3", reply)
	}
}

func TestHTTPHandlerBadRequest(t *testing.T) {
	n := NewNode(testConfig("node1", nil), &stubTransport{})
	srv := httptest.NewServer(NewHTTPHandler(n))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/raft/request_vote", "application/json", bytes.NewReader([]byte("{")))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
