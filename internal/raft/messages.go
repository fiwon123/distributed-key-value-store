// Package raft implements a minimal, educational Raft consensus layer.
//
// It covers leader election (§5.2), log replication, commit, and applying
// committed entries to a state machine (§5.3–5.4). Persistence and snapshots
// are the next phases. The implementation is deliberately simple — clarity over
// cleverness, and not production-ready.
package raft

// RequestVoteArgs is the RPC a candidate sends to request a vote (§5.2).
type RequestVoteArgs struct {
	Term         uint64 `json:"term"`
	CandidateID  string `json:"candidateId"`
	LastLogIndex uint64 `json:"lastLogIndex"`
	LastLogTerm  uint64 `json:"lastLogTerm"`
}

// RequestVoteReply is the response to RequestVoteArgs.
type RequestVoteReply struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"voteGranted"`
}

// LogEntry is a single entry in the replicated log (§5.3). Phase 2 sends
// heartbeats with no entries; Data carries the state-machine command once
// replication lands.
type LogEntry struct {
	Index uint64 `json:"index"`
	Term  uint64 `json:"term"`
	Data  []byte `json:"data,omitempty"`
}

// AppendEntriesArgs is the RPC a leader uses both for log replication and, with
// an empty Entries slice, as a heartbeat (§5.3).
type AppendEntriesArgs struct {
	Term         uint64     `json:"term"`
	LeaderID     string     `json:"leaderId"`
	PrevLogIndex uint64     `json:"prevLogIndex"`
	PrevLogTerm  uint64     `json:"prevLogTerm"`
	Entries      []LogEntry `json:"entries,omitempty"`
	LeaderCommit uint64     `json:"leaderCommit"`
}

// AppendEntriesReply is the response to AppendEntriesArgs.
type AppendEntriesReply struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
}
