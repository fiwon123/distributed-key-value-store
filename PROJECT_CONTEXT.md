# Project Context

## Identity

- **Project Name**: distributed-key-value-store
- **One-liner**: A minimal, educational distributed key-value store built from scratch with a hand-rolled Raft consensus algorithm.

## Problem

Most single-server key-value stores lose availability when the server fails. We need a simple replicated store where multiple nodes agree on the write order so that reads/writes remain consistent in the face of node failures (for educational purposes).

## Goal

Users should be able to run a small cluster (3 nodes by default) and perform key-value operations (PUT/GET/DELETE) against the system. In the MVP, a single-node instance is first made functional and tested; subsequent work adds elections, replication, and persistence with Raft to demonstrate how consensus keeps data consistent even if a leader fails.

## Stack

- **Backend**: Go (net/http or TCP API)
- **Consensus**: Hand-rolled Raft (educational, not production-ready)
- **Persistence**: Raft log (file-based) with snapshots planned
- **Containerization**: Docker Compose (dev sandbox now; 3-node cluster later)
- **Testing**: go test (unit tests; failure/chaos tests later)

## Architecture

```text
Client (curl/http client)
  ↓
HTTP API (node: PUT/GET/DELETE /v1/kv/{key}, GET /v1/status)
  ↓
State machine (in-memory KV map, evolves by applying committed log entries)
  ↓
Raft (leader election, log replication, commit/index, heartbeats)
  ↓
Persistent log (disk) + snapshots (later)
  ↕
Peer-to-peer RPC between nodes (follower/candidate/leader)
```

## Solution

Build a hand-rolled Raft implementation and a small HTTP API layer that drives the Raft log. Replicas must agree on write order; committed entries apply to an in-memory key-value state machine. Leader handles writes; reads may go to leader (MVP choice). On failure, new leader is elected via Raft and committed state remains available.

### MVP Features

- Single-node working first: start node, accept PUT/GET/DELETE, serve status
- Raft roles: Follower/Candidate/Leader
- Leader election (timeouts + majority)
- Log replication (append entries, match/next indices as time permits)
- Commit + apply (apply only committed entries)
- In-memory KV state machine (map[string]string)
- Basic persistence of log entries (optional early, required before "persistence" phase)
- Snapshot after replay/restart (planned)
- 3-node Docker Compose for local cluster

## API

| Method | Path | Purpose |
|---|---|---|
| PUT | /v1/kv/{key} | Set key = value (body or JSON) |
| GET | /v1/kv/{key} | Retrieve value (returns 404 if not found) |
| DELETE | /v1/kv/{key} | Delete key (returns 204 or 200) |
| GET | /v1/status | Return node state (role, term, leader, commit index, last applied, peer count)

## Design Choices

- **Educational, not production-ready**: prioritize clarity over performance; no TLS, no auth, minimal error handling initially.
- **No sharding**: single keyspace replicated to all nodes (3-node quorum).
- **Log is the WAL**: all state changes go through Raft log; applying committed log entries updates the KV map.
- **Leader reads (MVP)**: reads go to leader to avoid stale reads in earliest stages; can be revisited later.
- **Quorum loss stops writes**: if a leader cannot reach majority, writes are rejected (or blocked per Raft) — not best-effort.
- **Snapshots last**: implement log compaction/snapshots only after basic persistence and recovery are working.

## Build Order

1. **Single-node**: minimal HTTP server + in-memory KV map; no Raft yet (or stub) — just end-to-end tested.
2. **Elections**: Raft node skeleton, timers, election timeout, request vote, become leader/candidate/follower.
3. **Replication**: append entries, log matching, commit on majority, apply committed entries to KV.
4. **Persistence**: write/read log to disk, recover state on restart.
5. **Failure tests**: kill leader, verify new leader elected and committed values still readable (end-to-end).

## Topology

- **Components**: `.` (single component, flat Go layout)

## Key Commands

```bash
go mod tidy
go build ./...
go vet ./...
go test ./...
go run ./cmd/kvnode
make check          # local gate (Go steps skip until go.mod exists)
make dev-up         # start the dev sandbox (docker compose)
make dev-shell      # shell inside the sandbox (alias: make shell)
make dev-opencode   # run opencode inside the sandbox (alias: make opencode)
make dev-down       # stop the sandbox
```

## Notes

- Hand-rolled Raft for learning. Do not assume production safety or performance.
- Keep changes small and verifiable; follow workflow (issue → branch → PR → merge).
- No sharding. Replication is for availability/consistency under failure, not scale-out.
- This repo is prepared as setup-only skeleton + workflow; future sessions implement Raft step by step.
