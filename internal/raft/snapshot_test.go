package raft

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

// memState is a tiny state machine for snapshot tests: it records every applied
// command and can serialize/restore itself.
type memState struct {
	mu    sync.Mutex
	items []string
}

func (s *memState) apply(_ uint64, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, string(data))
	return nil
}

func (s *memState) snapshot() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(s.items)
}

func (s *memState) restore(data []byte) error {
	var items []string
	if len(data) > 0 {
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = items
	return nil
}

func (s *memState) contents() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.items...)
}

func snapshotTestConfig(id, dir string, state *memState) Config {
	cfg := testConfig(id, nil)
	cfg.Storage = NewFileStorage(dir)
	cfg.SnapshotThreshold = 3
	cfg.ApplyFunc = state.apply
	cfg.SnapshotFunc = state.snapshot
	cfg.RestoreFunc = state.restore
	return cfg
}

// --- storage -----------------------------------------------------------------

func TestFileStorageSnapshotRoundTrip(t *testing.T) {
	f := NewFileStorage(t.TempDir())

	meta, data, err := f.LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot (empty): %v", err)
	}
	if meta != (SnapshotMeta{}) || data != nil {
		t.Fatalf("empty LoadSnapshot = %+v, %q; want zero values", meta, data)
	}

	want := SnapshotMeta{Index: 7, Term: 3}
	if err := f.SaveSnapshot(want, []byte("image")); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	gotMeta, gotData, err := f.LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if gotMeta != want {
		t.Fatalf("meta = %+v, want %+v", gotMeta, want)
	}
	if string(gotData) != "image" {
		t.Fatalf("data = %q, want %q", gotData, "image")
	}
}

func TestFileStorageCompactTo(t *testing.T) {
	f := NewFileStorage(t.TempDir())
	entries := []LogEntry{
		{Index: 1, Term: 1},
		{Index: 2, Term: 1},
		{Index: 3, Term: 2},
		{Index: 4, Term: 2},
	}
	if err := f.AppendEntries(entries); err != nil {
		t.Fatalf("AppendEntries: %v", err)
	}
	if err := f.CompactTo(2); err != nil {
		t.Fatalf("CompactTo: %v", err)
	}

	_, got, err := f.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := entries[2:] // keep Index > 2
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries after compact = %+v, want %+v", got, want)
	}
}

// --- node snapshots ----------------------------------------------------------

func TestNodeSnapshotsAndCompactsLog(t *testing.T) {
	dir := t.TempDir()
	state := &memState{}
	n, err := Open(snapshotTestConfig("n1", dir, state), &stubTransport{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	forceLeader(n)

	for i := 0; i < 3; i++ {
		if err := n.Propose([]byte(fmt.Sprintf("v%d", i))); err != nil {
			t.Fatalf("Propose %d: %v", i, err)
		}
	}

	n.mu.Lock()
	start, included, applied, last := n.logStart, n.lastIncludedIndex, n.lastApplied, n.lastLogIndex()
	logLen := len(n.log)
	n.mu.Unlock()

	if start != 3 || included != 3 {
		t.Fatalf("logStart=%d lastIncluded=%d, want 3/3 (threshold reached)", start, included)
	}
	if applied != 3 || last != 3 {
		t.Fatalf("applied=%d lastLogIndex=%d, want 3/3", applied, last)
	}
	if logLen != 1 {
		t.Fatalf("in-memory log holds %d entries, want only the sentinel at 3", logLen)
	}

	// The snapshot is durable and the on-disk log prefix is gone.
	meta, data, err := NewFileStorage(dir).LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if meta.Index != 3 || meta.Term != 1 {
		t.Fatalf("snapshot meta = %+v, want index 3 term 1", meta)
	}
	var items []string
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("decode snapshot data: %v", err)
	}
	if want := []string{"v0", "v1", "v2"}; !reflect.DeepEqual(items, want) {
		t.Fatalf("snapshot data = %v, want %v", items, want)
	}
	if _, diskEntries, err := NewFileStorage(dir).Load(); err != nil {
		t.Fatalf("Load log: %v", err)
	} else if len(diskEntries) != 0 {
		t.Fatalf("on-disk log has %+v, want no entries after compaction", diskEntries)
	}
}

func TestNodeRecoversFromSnapshotAndTail(t *testing.T) {
	dir := t.TempDir()
	state := &memState{}
	cfg := snapshotTestConfig("n1", dir, state)
	cfg.SnapshotThreshold = 2

	n, err := Open(cfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	forceLeader(n)
	for i := 0; i < 3; i++ {
		if err := n.Propose([]byte(fmt.Sprintf("v%d", i))); err != nil {
			t.Fatalf("Propose %d: %v", i, err)
		}
	}

	// A fresh process on the same data dir starts from an empty state machine.
	fresh := &memState{}
	cfg.ApplyFunc = fresh.apply
	cfg.SnapshotFunc = fresh.snapshot
	cfg.RestoreFunc = fresh.restore
	n2, err := Open(cfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open (recover): %v", err)
	}

	// The snapshot at index 2 restores v0/v1; the entry beyond it is reloaded
	// but not yet committed, so it is not applied.
	if got, want := fresh.contents(), []string{"v0", "v1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restored state = %v, want %v", got, want)
	}
	n2.mu.Lock()
	commit, applied, start := n2.commitIndex, n2.lastApplied, n2.logStart
	tail, ok := n2.entryAt(3)
	n2.mu.Unlock()
	if commit != 2 || applied != 2 || start != 2 {
		t.Fatalf("recovered commit=%d applied=%d logStart=%d, want 2/2/2", commit, applied, start)
	}
	if !ok || string(tail.Data) != "v2" {
		t.Fatalf("recovered tail = %+v (ok=%v), want data v2", tail, ok)
	}

	// Re-committing the tail applies it on top of the restored snapshot.
	forceLeader(n2)
	if err := n2.Propose([]byte("v3")); err != nil {
		t.Fatalf("Propose after recover: %v", err)
	}
	if got, want := fresh.contents(), []string{"v0", "v1", "v2", "v3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state after re-commit = %v, want %v", got, want)
	}
}

func TestEntryAtBoundsAfterCompaction(t *testing.T) {
	state := &memState{}
	cfg := snapshotTestConfig("n1", t.TempDir(), state)
	cfg.SnapshotThreshold = 2
	n, err := Open(cfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	forceLeader(n)
	for i := 0; i < 2; i++ {
		if err := n.Propose([]byte("v")); err != nil {
			t.Fatalf("Propose %d: %v", i, err)
		}
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.entryAt(1); ok {
		t.Fatal("entryAt(1) should be false once compacted into the snapshot")
	}
	if _, ok := n.entryAt(2); !ok {
		t.Fatal("entryAt(2) should find the sentinel at the snapshot boundary")
	}
	if _, ok := n.entryAt(3); ok {
		t.Fatal("entryAt(3) beyond the log should be false")
	}
}

func TestOpenDropsLogCoveredBySnapshot(t *testing.T) {
	// Simulate a crash between saving the snapshot and truncating the log: the
	// snapshot covers entries 1..2 but the log still holds 1..3.
	dir := t.TempDir()
	f := NewFileStorage(dir)
	data, err := json.Marshal([]string{"a", "b"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := f.SaveSnapshot(SnapshotMeta{Index: 2, Term: 1}, data); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := f.AppendEntries([]LogEntry{
		{Index: 1, Term: 1, Data: []byte("a")},
		{Index: 2, Term: 1, Data: []byte("b")},
		{Index: 3, Term: 1, Data: []byte("c")},
	}); err != nil {
		t.Fatalf("AppendEntries: %v", err)
	}

	state := &memState{}
	cfg := testConfig("n1", nil)
	cfg.Storage = f
	cfg.ApplyFunc = state.apply
	cfg.SnapshotFunc = state.snapshot
	cfg.RestoreFunc = state.restore
	n, err := Open(cfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if got, want := state.contents(), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restored state = %v, want %v", got, want)
	}
	n.mu.Lock()
	start, logLen := n.logStart, len(n.log)
	tail, ok := n.entryAt(3)
	n.mu.Unlock()
	if start != 2 || logLen != 2 {
		t.Fatalf("recovered logStart=%d logLen=%d, want 2/2 (sentinel + index 3)", start, logLen)
	}
	if !ok || string(tail.Data) != "c" {
		t.Fatalf("recovered tail = %+v (ok=%v), want data c", tail, ok)
	}
}

func TestOpenRequiresRestoreFuncForSnapshot(t *testing.T) {
	dir := t.TempDir()
	if err := NewFileStorage(dir).SaveSnapshot(SnapshotMeta{Index: 4, Term: 2}, []byte("image")); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	cfg := testConfig("n1", nil)
	cfg.Storage = NewFileStorage(dir)
	if _, err := Open(cfg, &stubTransport{}); err == nil {
		t.Fatal("Open with a snapshot but no RestoreFunc should fail")
	}
}
