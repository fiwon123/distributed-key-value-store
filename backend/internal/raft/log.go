package raft

import "log"

// The methods in this file assume the caller holds n.mu unless the name says
// otherwise.

// lastLogIndex returns the index of the last log entry, or logStart when the log
// holds only its sentinel (logStart is 0 before any compaction). Caller must
// hold n.mu.
func (n *Node) lastLogIndex() uint64 {
	return n.logStart + uint64(len(n.log)-1)
}

// lastLogTerm returns the term of the last log entry, or the snapshot term when
// the log holds only its sentinel. Caller must hold n.mu.
func (n *Node) lastLogTerm() uint64 {
	if len(n.log) == 0 {
		return 0
	}
	return n.log[len(n.log)-1].Term
}

// entryAt returns the log entry at index. The boolean is false when index falls
// outside the in-memory log, e.g. because it was compacted into a snapshot.
// Caller must hold n.mu.
func (n *Node) entryAt(index uint64) (LogEntry, bool) {
	if index < n.logStart || index > n.lastLogIndex() {
		return LogEntry{}, false
	}
	return n.log[index-n.logStart], true
}

// advanceCommitLocked moves the commit index forward when a majority of nodes
// have stored an entry, and applies the newly committed entries. Per §5.4.2 a
// leader only commits entries from its own term directly; earlier entries
// commit indirectly once a current-term entry commits. Caller must hold n.mu.
func (n *Node) advanceCommitLocked() {
	for index := n.lastLogIndex(); index > n.commitIndex; index-- {
		entry, ok := n.entryAt(index)
		if !ok || entry.Term != n.term {
			break
		}
		acked := 1 // the leader itself
		for _, match := range n.matchIndex {
			if match >= index {
				acked++
			}
		}
		if acked >= n.majority() {
			n.commitIndex = index
			n.applyLocked()
			return
		}
	}
}

// applyLocked applies every committed-but-not-yet-applied entry, in order.
// Caller must hold n.mu.
func (n *Node) applyLocked() {
	for n.lastApplied < n.commitIndex {
		index := n.lastApplied + 1
		entry, ok := n.entryAt(index)
		if !ok {
			// The entry was compacted away underneath us; nothing left to apply.
			break
		}
		n.lastApplied = index
		if n.applyFn == nil {
			continue
		}
		if err := n.applyFn(entry.Index, entry.Data); err != nil {
			log.Printf("raft %s: apply entry %d failed: %v", n.id, entry.Index, err)
		}
	}
	n.maybeSnapshotLocked()
}
