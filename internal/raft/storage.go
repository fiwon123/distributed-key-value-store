package raft

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// HardState is the part of Raft state that must survive a crash (§5.1): the
// current term and the candidate this node voted for in that term.
type HardState struct {
	Term     uint64 `json:"term"`
	VotedFor string `json:"votedFor,omitempty"`
}

// Storage persists a node's durable state. It is not goroutine-safe; the
// enclosing Node serialises access with its own lock.
//
// Index 0 is a sentinel and is never persisted. Entries are addressed by their
// Index field, which always equals their position in the log.
type Storage interface {
	// SaveHardState durably records currentTerm and votedFor.
	SaveHardState(state HardState) error
	// AppendEntries durably appends entries to the end of the log.
	AppendEntries(entries []LogEntry) error
	// TruncateFrom removes every persisted entry with Index >= index.
	TruncateFrom(index uint64) error
	// Load returns the persisted hard state and log entries (excluding the
	// sentinel), or zero values if nothing has been persisted yet.
	Load() (HardState, []LogEntry, error)
}

// FileStorage stores a node's state under a directory: state.json holds the
// hard state (written atomically) and log holds length-prefixed JSON records.
type FileStorage struct {
	dir string
}

const (
	stateFileName = "state.json"
	logFileName   = "log"
)

// NewFileStorage returns a FileStorage rooted at dir. The directory is created
// on first write.
func NewFileStorage(dir string) *FileStorage { return &FileStorage{dir: dir} }

func (f *FileStorage) statePath() string { return filepath.Join(f.dir, stateFileName) }
func (f *FileStorage) logPath() string   { return filepath.Join(f.dir, logFileName) }

// SaveHardState writes term/votedFor atomically: temp file, fsync, rename over
// state.json.
func (f *FileStorage) SaveHardState(state HardState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("raft: encode hard state: %w", err)
	}
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		return fmt.Errorf("raft: create data dir: %w", err)
	}
	if err := writeFileAtomic(f.statePath(), payload); err != nil {
		return fmt.Errorf("raft: save hard state: %w", err)
	}
	return nil
}

// AppendEntries appends records to the log file and fsyncs before returning.
func (f *FileStorage) AppendEntries(entries []LogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		return fmt.Errorf("raft: create data dir: %w", err)
	}
	file, err := os.OpenFile(f.logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("raft: open log: %w", err)
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, entry := range entries {
		if err := writeRecord(w, entry); err != nil {
			return fmt.Errorf("raft: write log entry %d: %w", entry.Index, err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("raft: flush log: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("raft: sync log: %w", err)
	}
	return nil
}

// TruncateFrom rewrites the log without entries whose Index >= index.
func (f *FileStorage) TruncateFrom(index uint64) error {
	entries, err := f.loadLog()
	if err != nil {
		return err
	}
	var kept []LogEntry
	for _, entry := range entries {
		if entry.Index < index {
			kept = append(kept, entry)
		}
	}
	return f.rewriteLog(kept)
}

// Load reads the hard state and log. A missing file is not an error.
func (f *FileStorage) Load() (HardState, []LogEntry, error) {
	state, err := f.loadHardState()
	if err != nil {
		return HardState{}, nil, err
	}
	entries, err := f.loadLog()
	if err != nil {
		return HardState{}, nil, err
	}
	return state, entries, nil
}

func (f *FileStorage) loadHardState() (HardState, error) {
	payload, err := os.ReadFile(f.statePath())
	if os.IsNotExist(err) {
		return HardState{}, nil
	}
	if err != nil {
		return HardState{}, fmt.Errorf("raft: read hard state: %w", err)
	}
	var state HardState
	if err := json.Unmarshal(payload, &state); err != nil {
		return HardState{}, fmt.Errorf("raft: decode hard state: %w", err)
	}
	return state, nil
}

// loadLog reads records until EOF or a torn/invalid tail, which ends the log.
func (f *FileStorage) loadLog() ([]LogEntry, error) {
	file, err := os.Open(f.logPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("raft: open log: %w", err)
	}
	defer file.Close()

	var entries []LogEntry
	reader := bufio.NewReader(file)
	for {
		entry, err := readRecord(reader)
		if err != nil {
			// io.EOF is a clean end; any other error is a torn tail — stop at
			// the last complete record (the entry is uncommitted anyway).
			break
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// rewriteLog atomically replaces the log file with entries.
func (f *FileStorage) rewriteLog(entries []LogEntry) error {
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		return fmt.Errorf("raft: create data dir: %w", err)
	}
	tmp, err := os.CreateTemp(f.dir, logFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("raft: create temp log: %w", err)
	}
	defer os.Remove(tmp.Name())

	w := bufio.NewWriter(tmp)
	for _, entry := range entries {
		if err := writeRecord(w, entry); err != nil {
			return fmt.Errorf("raft: rewrite entry %d: %w", entry.Index, err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("raft: flush temp log: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("raft: sync temp log: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("raft: close temp log: %w", err)
	}
	if err := os.Rename(tmp.Name(), f.logPath()); err != nil {
		return fmt.Errorf("raft: replace log: %w", err)
	}
	return nil
}

// writeRecord writes a 4-byte big-endian length followed by the JSON entry.
func writeRecord(w io.Writer, entry LogEntry) error {
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

// readRecord reads one length-prefixed record. It returns io.EOF at a clean
// end and io.ErrUnexpectedEOF for a partial (torn) record.
func readRecord(r io.Reader) (LogEntry, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return LogEntry{}, err
	}
	payload := make([]byte, binary.BigEndian.Uint32(header[:]))
	if _, err := io.ReadFull(r, payload); err != nil {
		return LogEntry{}, err
	}
	var entry LogEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return LogEntry{}, err
	}
	return entry, nil
}

// writeFileAtomic writes payload to path via a temp file + fsync + rename.
func writeFileAtomic(path string, payload []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(payload); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
