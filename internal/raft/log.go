package raft

import "log"

// The methods in this file assume the caller holds n.mu unless the name says
// otherwise.

// lastLogIndex returns the index of the last log entry (0 for an empty log,
// which holds only the sentinel). Caller must hold n.mu.
func (n *Node) lastLogIndex() uint64 {
	return uint64(len(n.log) - 1)
}

// lastLogTerm returns the term of the last log entry. Caller must hold n.mu.
func (n *Node) lastLogTerm() uint64 {
	if len(n.log) < 2 {
		return 0
	}
	return n.log[len(n.log)-1].Term
}

// advanceCommitLocked moves the commit index forward when a majority of nodes
// have stored an entry, and applies the newly committed entries. Per §5.4.2 a
// leader only commits entries from its own term directly; earlier entries
// commit indirectly once a current-term entry commits. Caller must hold n.mu.
func (n *Node) advanceCommitLocked() {
	for index := n.lastLogIndex(); index > n.commitIndex; index-- {
		if n.log[index].Term != n.term {
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
		n.lastApplied++
		entry := n.log[n.lastApplied]
		if n.applyFn == nil {
			continue
		}
		if err := n.applyFn(entry.Index, entry.Data); err != nil {
			log.Printf("raft %s: apply entry %d failed: %v", n.id, entry.Index, err)
		}
	}
}
