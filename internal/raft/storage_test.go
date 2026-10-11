package raft

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFileStorageRoundTrip(t *testing.T) {
	f := NewFileStorage(t.TempDir())

	if err := f.SaveHardState(HardState{Term: 5, VotedFor: "node2"}); err != nil {
		t.Fatalf("SaveHardState: %v", err)
	}
	entries := []LogEntry{
		{Index: 1, Term: 5, Data: []byte("a")},
		{Index: 2, Term: 5, Data: []byte("b")},
	}
	if err := f.AppendEntries(entries); err != nil {
		t.Fatalf("AppendEntries: %v", err)
	}

	state, got, err := f.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state != (HardState{Term: 5, VotedFor: "node2"}) {
		t.Fatalf("hard state = %+v, want term 5 votedFor node2", state)
	}
	if !reflect.DeepEqual(got, entries) {
		t.Fatalf("entries = %+v, want %+v", got, entries)
	}
}

func TestFileStorageEmpty(t *testing.T) {
	state, entries, err := NewFileStorage(t.TempDir()).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state != (HardState{}) || len(entries) != 0 {
		t.Fatalf("Load = %+v, %v; want zero values", state, entries)
	}
}

func TestFileStorageDropsTornTail(t *testing.T) {
	f := NewFileStorage(t.TempDir())
	if err := f.AppendEntries([]LogEntry{{Index: 1, Term: 1, Data: []byte("a")}}); err != nil {
		t.Fatalf("AppendEntries: %v", err)
	}

	// Simulate a crash mid-write: a length header claiming 9 bytes with only 2
	// present. The reader must stop at the last complete record.
	file, err := os.OpenFile(filepath.Join(f.dir, logFileName), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	if _, err := file.Write([]byte{0, 0, 0, 9, '{', '"'}); err != nil {
		t.Fatalf("write partial: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, entries, err := f.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 1 || entries[0].Index != 1 {
		t.Fatalf("entries = %+v, want just index 1", entries)
	}
}

func TestFileStorageTruncateFrom(t *testing.T) {
	f := NewFileStorage(t.TempDir())
	if err := f.AppendEntries([]LogEntry{
		{Index: 1, Term: 1, Data: []byte("a")},
		{Index: 2, Term: 1, Data: []byte("b")},
		{Index: 3, Term: 1, Data: []byte("c")},
	}); err != nil {
		t.Fatalf("AppendEntries: %v", err)
	}
	if err := f.TruncateFrom(2); err != nil {
		t.Fatalf("TruncateFrom: %v", err)
	}

	_, got, err := f.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 || got[0].Index != 1 {
		t.Fatalf("entries after truncate = %+v, want only index 1", got)
	}
}

func TestNodeRecoversPersistedState(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig("n1", nil)
	cfg.Storage = NewFileStorage(dir)

	n1, err := Open(cfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open n1: %v", err)
	}
	forceLeader(n1)
	if err := n1.Propose([]byte("value")); err != nil {
		t.Fatalf("Propose: %v", err)
	}

	// A fresh process on the same data dir must recover term, vote, and log.
	n2, err := Open(cfg, &stubTransport{})
	if err != nil {
		t.Fatalf("Open n2: %v", err)
	}
	if n2.term != n1.term {
		t.Fatalf("recovered term = %d, want %d", n2.term, n1.term)
	}
	if n2.votedFor != n1.votedFor {
		t.Fatalf("recovered votedFor = %q, want %q", n2.votedFor, n1.votedFor)
	}
	if n2.lastLogIndex() != 1 {
		t.Fatalf("recovered log index = %d, want 1", n2.lastLogIndex())
	}
	if got := string(n2.log[1].Data); got != "value" {
		t.Fatalf("recovered entry data = %q, want %q", got, "value")
	}
}

func TestFileStorageMissingDirRoundTrip(t *testing.T) {
	// NewFileStorage should create the directory on first write.
	dir := filepath.Join(t.TempDir(), "data")
	f := NewFileStorage(dir)
	if err := f.AppendEntries([]LogEntry{{Index: 1, Term: 1}}); err != nil {
		t.Fatalf("AppendEntries into missing dir: %v", err)
	}
	if _, _, err := f.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}
