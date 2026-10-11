package raft

import "log"

// SnapshotMeta identifies the last log entry captured in a snapshot. The state
// machine image is opaque to raft; Index/Term are what the log needs to resume.
type SnapshotMeta struct {
	Index uint64 `json:"index"`
	Term  uint64 `json:"term"`
}

// maybeSnapshotLocked takes a snapshot and compacts the log once enough entries
// have been applied since the last snapshot. It is a no-op when snapshots are
// disabled (no SnapshotFunc or threshold 0). Caller must hold n.mu.
func (n *Node) maybeSnapshotLocked() {
	if n.snapshotFn == nil || n.cfg.SnapshotThreshold == 0 {
		return
	}
	if n.lastApplied < n.lastIncludedIndex+n.cfg.SnapshotThreshold {
		return
	}
	entry, ok := n.entryAt(n.lastApplied)
	if !ok {
		return
	}
	data, err := n.snapshotFn()
	if err != nil {
		log.Printf("raft %s: capture snapshot at %d: %v", n.id, n.lastApplied, err)
		return
	}
	meta := SnapshotMeta{Index: n.lastApplied, Term: entry.Term}
	if n.storage != nil {
		if err := n.storage.SaveSnapshot(meta, data); err != nil {
			log.Printf("raft %s: save snapshot at %d: %v", n.id, meta.Index, err)
			return
		}
	}
	n.compactLocked(meta)
}

// compactLocked drops every log entry at or below meta.Index, keeping a sentinel
// entry at meta.Index so the log stays contiguous. Caller must hold n.mu.
func (n *Node) compactLocked(meta SnapshotMeta) {
	kept := make([]LogEntry, 1, len(n.log))
	kept[0] = LogEntry{Index: meta.Index, Term: meta.Term}
	for _, entry := range n.log {
		if entry.Index > meta.Index {
			kept = append(kept, entry)
		}
	}
	n.log = kept
	n.logStart = meta.Index
	n.lastIncludedIndex = meta.Index
	n.lastIncludedTerm = meta.Term
	if n.storage != nil {
		if err := n.storage.CompactTo(meta.Index); err != nil {
			log.Printf("raft %s: compact log to %d: %v", n.id, meta.Index, err)
		}
	}
}
