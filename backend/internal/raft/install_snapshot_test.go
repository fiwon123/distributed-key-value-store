package raft

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
)

// --- follower side of InstallSnapshot ----------------------------------------

func TestHandleInstallSnapshotRestoresAndCompacts(t *testing.T) {
	dir := t.TempDir()
	state := &memState{}
	cfg := testConfig("n1", nil)
	cfg.Storage = NewFileStorage(dir)
	cfg.ApplyFunc = state.apply
	cfg.SnapshotFunc = state.snapshot
	cfg.RestoreFunc = state.restore
	n, err := Open(cfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Seed a committed prefix, then supersede it with a snapshot.
	n.HandleAppendEntries(AppendEntriesArgs{
		Term: 1, LeaderID: "n2", LeaderCommit: 2,
		Entries: []LogEntry{
			{Index: 1, Term: 1, Data: []byte("a")},
			{Index: 2, Term: 1, Data: []byte("b")},
		},
	})

	image, err := json.Marshal([]string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("marshal image: %v", err)
	}
	reply := n.HandleInstallSnapshot(InstallSnapshotArgs{
		Term: 1, LeaderID: "n2",
		LastIncludedIndex: 3, LastIncludedTerm: 1, Data: image,
	})
	if reply.Term != 1 {
		t.Fatalf("reply term = %d, want 1", reply.Term)
	}

	if got, want := state.contents(), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restored state = %v, want %v", got, want)
	}
	n.mu.Lock()
	start, included, commit, applied, logLen := n.logStart, n.lastIncludedIndex, n.commitIndex, n.lastApplied, len(n.log)
	n.mu.Unlock()
	if start != 3 || included != 3 {
		t.Fatalf("logStart=%d lastIncluded=%d, want 3/3", start, included)
	}
	if commit != 3 || applied != 3 {
		t.Fatalf("commit=%d applied=%d, want 3/3", commit, applied)
	}
	if logLen != 1 {
		t.Fatalf("in-memory log holds %d entries, want only the sentinel at 3", logLen)
	}

	// The snapshot is durable and the superseded log prefix is gone from disk.
	meta, data, err := NewFileStorage(dir).LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if meta != (SnapshotMeta{Index: 3, Term: 1}) {
		t.Fatalf("snapshot meta = %+v, want index 3 term 1", meta)
	}
	if string(data) != string(image) {
		t.Fatalf("snapshot data = %q, want %q", data, image)
	}
	if _, diskEntries, err := NewFileStorage(dir).Load(); err != nil {
		t.Fatalf("Load log: %v", err)
	} else if len(diskEntries) != 0 {
		t.Fatalf("on-disk log = %+v, want no entries after compaction", diskEntries)
	}
}

func TestHandleInstallSnapshotIgnoresStaleSnapshot(t *testing.T) {
	state := &memState{}
	cfg := testConfig("n1", nil)
	cfg.ApplyFunc = state.apply
	cfg.SnapshotFunc = state.snapshot
	cfg.RestoreFunc = state.restore
	n := NewNode(cfg, &stubTransport{})

	imageA, _ := json.Marshal([]string{"a"})
	n.HandleInstallSnapshot(InstallSnapshotArgs{
		Term: 1, LeaderID: "n2", LastIncludedIndex: 5, LastIncludedTerm: 1, Data: imageA,
	})

	// A snapshot at or below our boundary must be ignored, even with a newer
	// image — the state machine keeps the first one.
	imageB, _ := json.Marshal([]string{"x", "y"})
	reply := n.HandleInstallSnapshot(InstallSnapshotArgs{
		Term: 1, LeaderID: "n2", LastIncludedIndex: 5, LastIncludedTerm: 1, Data: imageB,
	})
	if reply.Term != 1 {
		t.Fatalf("reply term = %d, want 1", reply.Term)
	}
	if got, want := state.contents(), []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state = %v, want unchanged %v", got, want)
	}
	// An older index is ignored too.
	n.HandleInstallSnapshot(InstallSnapshotArgs{
		Term: 1, LeaderID: "n2", LastIncludedIndex: 2, LastIncludedTerm: 1, Data: imageB,
	})
	if got, want := state.contents(), []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state after older snapshot = %v, want %v", got, want)
	}

	// A stale term is rejected without touching state.
	reply = n.HandleInstallSnapshot(InstallSnapshotArgs{
		Term: 0, LeaderID: "n9", LastIncludedIndex: 9, LastIncludedTerm: 1, Data: imageB,
	})
	if reply.Term != 1 {
		t.Fatalf("stale-term reply term = %d, want current term 1", reply.Term)
	}
	if got, want := state.contents(), []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state after stale term = %v, want %v", got, want)
	}
}

func TestHandleInstallSnapshotKeepsMatchingTail(t *testing.T) {
	state := &memState{}
	cfg := testConfig("n1", nil)
	cfg.ApplyFunc = state.apply
	cfg.RestoreFunc = state.restore
	n := NewNode(cfg, &stubTransport{})

	// Follower already holds 1..4, all term 1.
	n.HandleAppendEntries(AppendEntriesArgs{
		Term: 1, LeaderID: "n2",
		Entries: []LogEntry{
			{Index: 1, Term: 1, Data: []byte("a")},
			{Index: 2, Term: 1, Data: []byte("b")},
			{Index: 3, Term: 1, Data: []byte("c")},
			{Index: 4, Term: 1, Data: []byte("d")},
		},
	})

	image, _ := json.Marshal([]string{"a", "b"})
	// The snapshot ends exactly on entry 2, which we already have with term 1,
	// so entries 3 and 4 are still valid and must be kept.
	n.HandleInstallSnapshot(InstallSnapshotArgs{
		Term: 1, LeaderID: "n2", LastIncludedIndex: 2, LastIncludedTerm: 1, Data: image,
	})

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.logStart != 2 {
		t.Fatalf("logStart = %d, want 2", n.logStart)
	}
	if n.lastLogIndex() != 4 {
		t.Fatalf("lastLogIndex = %d, want 4 (tail preserved)", n.lastLogIndex())
	}
	if e, ok := n.entryAt(3); !ok || string(e.Data) != "c" {
		t.Fatalf("entry 3 = %+v (ok=%v), want data c", e, ok)
	}
	if e, ok := n.entryAt(4); !ok || string(e.Data) != "d" {
		t.Fatalf("entry 4 = %+v (ok=%v), want data d", e, ok)
	}
}

func TestHandleInstallSnapshotDropsDivergentTailDurably(t *testing.T) {
	dir := t.TempDir()
	state := &memState{}
	cfg := testConfig("n1", nil)
	cfg.Storage = NewFileStorage(dir)
	cfg.ApplyFunc = state.apply
	cfg.RestoreFunc = state.restore
	n, err := Open(cfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Follower has an uncommitted divergent tail 1..4 in term 1.
	n.HandleAppendEntries(AppendEntriesArgs{
		Term: 1, LeaderID: "n2",
		Entries: []LogEntry{
			{Index: 1, Term: 1, Data: []byte("a")},
			{Index: 2, Term: 1, Data: []byte("b")},
			{Index: 3, Term: 1, Data: []byte("c")},
			{Index: 4, Term: 1, Data: []byte("d")},
		},
	})

	image, _ := json.Marshal([]string{"a", "b", "c"})
	// Snapshot at index 3 in term 2: our entry 3 is term 1, so nothing in the
	// existing log survives.
	n.HandleInstallSnapshot(InstallSnapshotArgs{
		Term: 2, LeaderID: "n3", LastIncludedIndex: 3, LastIncludedTerm: 2, Data: image,
	})

	n.mu.Lock()
	logLen, start, last := len(n.log), n.logStart, n.lastLogIndex()
	n.mu.Unlock()
	if logLen != 1 || start != 3 || last != 3 {
		t.Fatalf("log len=%d start=%d last=%d, want 1/3/3", logLen, start, last)
	}
	if _, diskEntries, err := NewFileStorage(dir).Load(); err != nil {
		t.Fatalf("Load: %v", err)
	} else if len(diskEntries) != 0 {
		t.Fatalf("on-disk log = %+v, want divergent tail removed", diskEntries)
	}
}

// --- leader send branch -------------------------------------------------------

func TestLeaderSendsSnapshotToLaggingFollower(t *testing.T) {
	state := &memState{}
	cfg := testConfig("n1", []Peer{{ID: "n2"}})
	cfg.Storage = NewFileStorage(t.TempDir())
	cfg.SnapshotThreshold = 100 // captures are driven manually below
	cfg.ApplyFunc = state.apply
	cfg.SnapshotFunc = state.snapshot
	cfg.RestoreFunc = state.restore

	var got InstallSnapshotArgs
	calls := 0
	stub := &stubTransport{
		appnd: func(_ Peer, _ AppendEntriesArgs) (AppendEntriesReply, error) {
			return AppendEntriesReply{Term: 1, Success: true}, nil
		},
		snap: func(_ Peer, args InstallSnapshotArgs) (InstallSnapshotReply, error) {
			calls++
			got = args
			return InstallSnapshotReply{Term: 1}, nil
		},
	}
	n := NewNode(cfg, stub)
	forceLeader(n)

	// Pretend the leader compacted a prefix into a snapshot at index 5, and its
	// follower is still at the start of the log.
	n.mu.Lock()
	n.lastIncludedIndex = 5
	n.lastIncludedTerm = 1
	n.logStart = 5
	n.log = []LogEntry{{Index: 5, Term: 1}}
	n.snapshotMeta = SnapshotMeta{Index: 5, Term: 1}
	n.snapshotData = []byte(`["x"]`)
	n.nextIndex[0] = 1
	n.matchIndex[0] = 0
	n.mu.Unlock()

	n.replicateAll()

	if calls != 1 {
		t.Fatalf("InstallSnapshot calls = %d, want 1", calls)
	}
	if got.LastIncludedIndex != 5 || got.LastIncludedTerm != 1 {
		t.Fatalf("sent snapshot meta = %d/%d, want 5/1", got.LastIncludedIndex, got.LastIncludedTerm)
	}
	if string(got.Data) != `["x"]` {
		t.Fatalf("sent snapshot data = %q, want %q", got.Data, `["x"]`)
	}
	n.mu.Lock()
	match, next := n.matchIndex[0], n.nextIndex[0]
	n.mu.Unlock()
	if match != 5 || next != 6 {
		t.Fatalf("after snapshot match=%d next=%d, want 5/6", match, next)
	}
}

func TestLeaderStepsDownOnHigherTermInstallSnapshotReply(t *testing.T) {
	cfg := testConfig("n1", []Peer{{ID: "n2"}})
	cfg.SnapshotThreshold = 100
	n := NewNode(cfg, &stubTransport{
		snap: func(_ Peer, _ InstallSnapshotArgs) (InstallSnapshotReply, error) {
			return InstallSnapshotReply{Term: 9}, nil
		},
	})
	forceLeader(n)
	n.mu.Lock()
	n.lastIncludedIndex = 3
	n.lastIncludedTerm = 1
	n.logStart = 3
	n.log = []LogEntry{{Index: 3, Term: 1}}
	n.snapshotMeta = SnapshotMeta{Index: 3, Term: 1}
	n.snapshotData = []byte("img")
	n.nextIndex[0] = 1
	n.mu.Unlock()

	n.replicateAll()

	snap := n.Status()
	if snap.Role != "follower" || snap.Term != 9 {
		t.Fatalf("snapshot = %+v, want follower term 9", snap)
	}
}

// --- transport round trip -----------------------------------------------------

func TestHTTPInstallSnapshotRoundTrip(t *testing.T) {
	state := &memState{}
	cfg := testConfig("n1", nil)
	cfg.ApplyFunc = state.apply
	cfg.RestoreFunc = state.restore
	n := NewNode(cfg, &stubTransport{})
	srv := httptest.NewServer(NewHTTPHandler(n))
	defer srv.Close()

	image, _ := json.Marshal([]string{"a", "b"})
	reply, err := NewHTTPTransport().InstallSnapshot(context.Background(), Peer{ID: "n1", URL: srv.URL}, InstallSnapshotArgs{
		Term: 2, LeaderID: "n2", LastIncludedIndex: 2, LastIncludedTerm: 2, Data: image,
	})
	if err != nil {
		t.Fatalf("InstallSnapshot: %v", err)
	}
	if reply.Term != 2 {
		t.Fatalf("reply term = %d, want 2", reply.Term)
	}
	if got, want := state.contents(), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restored state = %v, want %v", got, want)
	}
}
