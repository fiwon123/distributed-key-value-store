package raft

import "context"

// HandleAppendEntries implements the follower side of §5.3: term rules, the
// prev-log consistency check, conflict truncation, and advancing the follower's
// commit index from the leader's.
func (n *Node) HandleAppendEntries(args AppendEntriesArgs) AppendEntriesReply {
	n.mu.Lock()
	defer n.mu.Unlock()

	if args.Term < n.term {
		return AppendEntriesReply{Term: n.term, Success: false}
	}
	if args.Term > n.term {
		n.term = args.Term
		n.votedFor = ""
	}
	n.role = Follower
	n.leaderID = args.LeaderID
	n.lastActivity = n.now()

	// Consistency check: we must have stored an entry matching the leader's
	// prev position.
	if args.PrevLogIndex > n.lastLogIndex() {
		return AppendEntriesReply{Term: n.term, Success: false}
	}
	if args.PrevLogIndex > 0 {
		prev, ok := n.entryAt(args.PrevLogIndex)
		if !ok || prev.Term != args.PrevLogTerm {
			return AppendEntriesReply{Term: n.term, Success: false}
		}
	}
	// Entries at or below our snapshot boundary are already captured; storing
	// them again (or a gap beneath them) would corrupt the log. Reject so the
	// leader backs off until InstallSnapshot can catch us up.
	if len(args.Entries) > 0 && args.Entries[0].Index <= n.lastIncludedIndex {
		return AppendEntriesReply{Term: n.term, Success: false}
	}

	// Append entries, truncating a conflicting tail on the first mismatch.
	for i, entry := range args.Entries {
		if existing, ok := n.entryAt(entry.Index); ok {
			if existing.Term == entry.Term {
				continue // already have this entry
			}
			n.log = n.log[:entry.Index-n.logStart] // drop the divergent suffix
			n.persistTruncateLocked(entry.Index)
		}
		n.log = append(n.log, args.Entries[i:]...)
		n.persistEntriesLocked(args.Entries[i:])
		break
	}

	if args.LeaderCommit > n.commitIndex {
		n.commitIndex = args.LeaderCommit
		if last := n.lastLogIndex(); n.commitIndex > last {
			n.commitIndex = last
		}
		n.applyLocked()
	}
	return AppendEntriesReply{Term: n.term, Success: true}
}

// replicateAll pushes the leader's log to every peer, then advances the commit
// index on the leader. It is a no-op unless this node is the leader. Called
// from the heartbeat tick and from Propose.
func (n *Node) replicateAll() {
	n.mu.Lock()
	if n.role != Leader {
		n.mu.Unlock()
		return
	}
	term := n.term
	commit := n.commitIndex
	lastIndex := n.lastLogIndex()

	if len(n.peers) == 0 {
		// A single-node cluster reaches a majority on its own.
		n.advanceCommitLocked()
		n.mu.Unlock()
		return
	}

	type request struct {
		index int
		args  AppendEntriesArgs
	}
	type snapRequest struct {
		index int
		args  InstallSnapshotArgs
	}
	requests := make([]request, 0, len(n.peers))
	snapRequests := make([]snapRequest, 0, len(n.peers))
	for i := range n.peers {
		next := n.nextIndex[i]
		if next < 1 {
			next = 1
		}
		if next <= n.lastIncludedIndex {
			// The peer needs entries we already compacted into a snapshot. Send
			// the snapshot instead of the (unavailable) log prefix.
			if n.snapshotMeta.Index == 0 {
				continue // nothing to send yet
			}
			snapRequests = append(snapRequests, snapRequest{
				index: i,
				args: InstallSnapshotArgs{
					Term:              term,
					LeaderID:          n.id,
					LastIncludedIndex: n.lastIncludedIndex,
					LastIncludedTerm:  n.lastIncludedTerm,
					Data:              append([]byte(nil), n.snapshotData...),
				},
			})
			continue
		}
		prevIndex := next - 1
		var prevTerm uint64
		if prev, ok := n.entryAt(prevIndex); ok {
			prevTerm = prev.Term
		}
		var entries []LogEntry
		if next <= lastIndex {
			entries = append([]LogEntry(nil), n.log[next-n.logStart:]...)
		}
		requests = append(requests, request{
			index: i,
			args: AppendEntriesArgs{
				Term:         term,
				LeaderID:     n.id,
				PrevLogIndex: prevIndex,
				PrevLogTerm:  prevTerm,
				Entries:      entries,
				LeaderCommit: commit,
			},
		})
	}
	n.mu.Unlock()

	for _, req := range requests {
		reply, err := n.transport.AppendEntries(context.Background(), n.peers[req.index], req.args)
		if err != nil {
			continue
		}
		n.mu.Lock()
		if reply.Term > n.term {
			n.becomeFollowerLocked(reply.Term)
			n.mu.Unlock()
			continue
		}
		if n.role != Leader {
			n.mu.Unlock()
			continue
		}
		if reply.Success {
			match := req.args.PrevLogIndex + uint64(len(req.args.Entries))
			if match > n.matchIndex[req.index] {
				n.matchIndex[req.index] = match
			}
			n.nextIndex[req.index] = n.matchIndex[req.index] + 1
			n.advanceCommitLocked()
		} else if n.nextIndex[req.index] > 1 {
			// Back off and retry on the next heartbeat/propose.
			n.nextIndex[req.index]--
		}
		n.mu.Unlock()
	}

	// Ship the snapshot to peers that fell behind the compacted boundary.
	for _, req := range snapRequests {
		reply, err := n.transport.InstallSnapshot(context.Background(), n.peers[req.index], req.args)
		if err != nil {
			continue
		}
		n.mu.Lock()
		if reply.Term > n.term {
			n.becomeFollowerLocked(reply.Term)
			n.mu.Unlock()
			continue
		}
		if n.role != Leader {
			n.mu.Unlock()
			continue
		}
		if req.args.LastIncludedIndex > n.matchIndex[req.index] {
			n.matchIndex[req.index] = req.args.LastIncludedIndex
		}
		n.nextIndex[req.index] = n.matchIndex[req.index] + 1
		n.mu.Unlock()
	}
}
