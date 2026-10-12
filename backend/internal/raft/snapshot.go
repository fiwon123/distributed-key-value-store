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
	// Keep the image in memory so this node can ship it to a lagging follower
	// (InstallSnapshot) without invoking SnapshotFunc again.
	n.snapshotMeta = meta
	n.snapshotData = data
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

// HandleInstallSnapshot implements the follower side of §7: term rules, then
// restoring the state machine from the leader's snapshot, persisting and
// compacting the log up to the snapshot boundary, and advancing commit/apply to
// it. A snapshot at or below our own boundary is stale and ignored.
func (n *Node) HandleInstallSnapshot(args InstallSnapshotArgs) InstallSnapshotReply {
	n.mu.Lock()
	defer n.mu.Unlock()

	if args.Term < n.term {
		return InstallSnapshotReply{Term: n.term}
	}
	if args.Term > n.term {
		n.becomeFollowerLocked(args.Term)
	}
	// An InstallSnapshot only ever comes from the leader; adopt that view and
	// reset the election timer even when the term is unchanged.
	n.role = Follower
	n.leaderID = args.LeaderID
	n.lastActivity = n.now()

	// Ignore a snapshot we already have (or one the log already covers).
	if args.LastIncludedIndex <= n.lastIncludedIndex {
		return InstallSnapshotReply{Term: n.term}
	}

	// Rebuild the state machine from the leader's image.
	if n.restoreFn != nil {
		if err := n.restoreFn(args.Data); err != nil {
			log.Printf("raft %s: restore installed snapshot at %d: %v", n.id, args.LastIncludedIndex, err)
		}
	}

	// If our log already holds the entry the snapshot ends on with the same
	// term, its tail after that index is still valid and can be kept; otherwise
	// the whole log is superseded and replaced by a sentinel at the boundary.
	meta := SnapshotMeta{Index: args.LastIncludedIndex, Term: args.LastIncludedTerm}
	keepTail := false
	if entry, ok := n.entryAt(args.LastIncludedIndex); ok && entry.Term == args.LastIncludedTerm {
		keepTail = true
	}
	kept := []LogEntry{{Index: meta.Index, Term: meta.Term}}
	if keepTail {
		for _, entry := range n.log {
			if entry.Index > meta.Index {
				kept = append(kept, entry)
			}
		}
	} else if n.storage != nil {
		// Drop any divergent entries above the boundary so a restart does not
		// resurrect them from disk.
		n.persistTruncateLocked(meta.Index + 1)
	}

	if n.storage != nil {
		if err := n.storage.SaveSnapshot(meta, args.Data); err != nil {
			log.Printf("raft %s: save installed snapshot at %d: %v", n.id, meta.Index, err)
		}
		if err := n.storage.CompactTo(meta.Index); err != nil {
			log.Printf("raft %s: compact log to %d: %v", n.id, meta.Index, err)
		}
	}

	n.log = kept
	n.logStart = meta.Index
	n.lastIncludedIndex = meta.Index
	n.lastIncludedTerm = meta.Term
	n.snapshotMeta = meta
	n.snapshotData = append([]byte(nil), args.Data...)
	if n.commitIndex < meta.Index {
		n.commitIndex = meta.Index
	}
	if n.lastApplied < meta.Index {
		n.lastApplied = meta.Index
	}
	return InstallSnapshotReply{Term: n.term}
}
