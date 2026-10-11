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
	if args.PrevLogIndex > 0 && n.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		return AppendEntriesReply{Term: n.term, Success: false}
	}

	// Append entries, truncating a conflicting tail on the first mismatch.
	for i, entry := range args.Entries {
		if entry.Index <= n.lastLogIndex() {
			if n.log[entry.Index].Term == entry.Term {
				continue // already have this entry
			}
			n.log = n.log[:entry.Index] // drop the divergent suffix
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
	requests := make([]request, 0, len(n.peers))
	for i := range n.peers {
		next := n.nextIndex[i]
		if next < 1 {
			next = 1
		}
		prevIndex := next - 1
		var prevTerm uint64
		if prevIndex > 0 && prevIndex <= lastIndex {
			prevTerm = n.log[prevIndex].Term
		}
		var entries []LogEntry
		if next <= lastIndex {
			entries = append([]LogEntry(nil), n.log[next:]...)
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
}
