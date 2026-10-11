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

**MVP feature-complete (pre-release).** Implemented: single-node HTTP + in-memory
store, Raft leader election, log replication and commit, on-disk log persistence
with restart recovery, in-process leader-failure tests, and the cluster
dashboard. Remaining: log compaction/snapshots and a compose-based 3-node
cluster.

## Stack

- **Language**: Go (standard toolchain)
- **Consensus**: hand-rolled Raft (educational)
- **Persistence**: Raft log on disk; snapshots planned
- **Dashboard**: React + Vite + TypeScript, embedded in the node binary
- **Cluster**: Docker Compose (three local nodes) — planned
- **Testing**: standard `testing` package (`go test`); Vitest for the dashboard

## API

| Method | Path | Purpose |
|---|---|---|
| `PUT` | `/v1/kv/{key}` | Set `key` to a value (raw body, or JSON `{"value":"..."}`) |
| `GET` | `/v1/kv/{key}` | Read a value (404 if absent) |
| `DELETE` | `/v1/kv/{key}` | Delete a key (204, or 404 if absent) |
| `GET` | `/v1/status` | Node role, term, leader, commit index, last applied |
| `GET` | `/cluster` | Dashboard discovery: local node id + peer ids |

## Dashboard

Each node serves a small web dashboard at `/` (same origin as the API). It shows
every cluster node's role, term, leader, and commit/applied indices, and lets you
exercise `PUT`/`GET`/`DELETE` against the leader. The browser reaches peers
through a server-side `/node/{id}/` proxy, so nodes only need to be reachable
from each other — not from the browser.

Build it once; the node embeds the output:

```bash
make web-build    # vite build -> internal/webui/dist (embedded via go:embed)
go run ./cmd/kvnode -id node1 -http :8080
```

Without a build, the binary serves a committed placeholder page, so
`go build ./...` keeps working with no Node installed.

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

## Development

The docker compose **dev sandbox** gives you the full toolchain (Go, Node,
`gh`, `air` fast reload) with your host GitHub config mounted read-only and
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
make check           # all of the above (Go gate)

cd web
npm ci               # install frontend deps
npm run dev          # Vite dev server (proxies to a running node)
npm test             # frontend unit tests
make web-check       # frontend gate: ci + lint + format + test + build
```

The project follows an issue-first, branch-based workflow — see
[`WORKFLOW_PROMPT.md`](WORKFLOW_PROMPT.md) and [`AGENTS.md`](AGENTS.md).
