# distributed-key-value-store

A minimal, educational distributed key-value store built from scratch in Go, with
a hand-rolled Raft consensus algorithm.

> **Educational project — not production-ready.** There is no TLS, no
> authentication, and no operational hardening. The goal is to understand how
> consensus keeps replicated state consistent through node failures, not to run
> this in front of real traffic.

## Problem

A single-server key-value store loses availability when that server fails. To
keep reads and writes available across failures, several nodes must agree on the
order of writes. This project implements that agreement (Raft) from scratch and
drives a small key-value store with it.

## What you can do

Once built, you can run a small cluster (three nodes by default) and:

- store, read, and delete keys over HTTP (`PUT`, `GET`, `DELETE`);
- inspect a node's role and state (`GET /v1/status`);
- kill the current leader and watch a new one get elected, with committed data
  still readable.

## Status

**MVP in progress.** Build Order step 1 (single-node HTTP + in-memory store) is
implemented. Raft elections, replication, persistence, and failure tests land in
subsequent steps.

## Stack

- **Language**: Go (standard toolchain)
- **Consensus**: hand-rolled Raft (educational)
- **Persistence**: Raft log on disk; snapshots planned
- **Cluster**: Docker Compose (three local nodes) — planned
- **Testing**: standard `testing` package (`go test`)

## API

| Method | Path | Purpose |
|---|---|---|
| `PUT` | `/v1/kv/{key}` | Set `key` to a value (raw body, or JSON `{"value":"..."}`) |
| `GET` | `/v1/kv/{key}` | Read a value (404 if absent) |
| `DELETE` | `/v1/kv/{key}` | Delete a key (204, or 404 if absent) |
| `GET` | `/v1/status` | Node role, term, leader, commit index, last applied |

## Run a single node

```bash
go run ./cmd/kvnode -id node1 -http :8080

# in another shell
curl -i -X PUT --data-binary 'hello' http://localhost:8080/v1/kv/greeting
curl -i http://localhost:8080/v1/kv/greeting
curl -i -X DELETE http://localhost:8080/v1/kv/greeting
curl -i http://localhost:8080/v1/status
```

## Design choices

- **No sharding** — one keyspace, replicated to every node (3-node quorum).
- **The log is the write-ahead log** — every change goes through the Raft log;
  applying committed entries updates an in-memory map.
- **Leader reads (MVP)** — reads go to the leader to avoid stale reads early on.
- **Quorum loss stops writes** — a leader that cannot reach a majority rejects
  writes rather than best-effort accepting them.
- **Snapshots last** — log compaction only after basic persistence works.

## Build order

1. Single-node: HTTP server + in-memory map, fully tested
2. Leader election: roles, timeouts, majority
3. Log replication: append entries, commit, apply
4. Persistence: log to disk + recovery on restart
5. Failure tests: kill the leader, confirm a new one is elected and data survives

## Done when

You can run three nodes locally, write a key, kill the leader, and still read the
committed value from the newly elected leader.

## Development

The docker compose **dev sandbox** gives you the full toolchain (Go, `gh`,
`air` fast reload) with your host GitHub config mounted read-only and
`GH_TOKEN`/`GITHUB_TOKEN` forwarded at runtime — never baked into the image.

```bash
make dev-up        # build + start the sandbox (needs docker on the host)
make dev-restart   # restart after Dockerfile/deps changes
make shell         # shell inside the sandbox
make opencode      # run opencode inside the sandbox
make dev-down      # stop and remove the sandbox
make dev-log       # follow container logs
```

Direct toolchain (host or inside the sandbox):

```bash
go build ./...       # build
go test ./...        # test
go vet ./...         # static analysis
gofmt -l .           # formatting (empty output = clean)
make check           # all of the above
```

The project follows an issue-first, branch-based workflow — see
[`WORKFLOW_PROMPT.md`](WORKFLOW_PROMPT.md) and [`AGENTS.md`](AGENTS.md).
