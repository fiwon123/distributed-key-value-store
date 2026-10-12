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

**MVP complete.** Single-node KV, Raft leader election, log replication and
commit, on-disk log persistence with restart recovery, automatic snapshots and
log compaction, the `InstallSnapshot` RPC that catches up a follower behind the
compacted boundary, in-process leader-failure tests, the cluster dashboard, and a
runnable 3-node cluster (local processes or Docker Compose).

## Stack

- **Language**: Go (standard toolchain)
- **Consensus**: hand-rolled Raft (educational)
- **Persistence**: Raft log and state-machine snapshots on disk (log compaction)
- **Dashboard**: React + Vite + TypeScript, embedded in the node binary
- **Cluster**: 3 nodes via local processes or Docker Compose
- **Testing**: standard `testing` package (`go test`); Vitest for the dashboard

## API

| Method | Path | Purpose |
|---|---|---|
| `PUT` | `/v1/kv/{key}` | Set `key` to a value (raw body, or JSON `{"value":"..."}`) |
| `GET` | `/v1/kv/{key}` | Read a value (404 if absent) |
| `DELETE` | `/v1/kv/{key}` | Delete a key (204, or 404 if absent) |
| `GET` | `/v1/status` | Node role, term, leader, commit index, last applied |
| `GET` | `/cluster` | Dashboard discovery: local node id + peer ids |
| `GET` | `/v1/openapi.json` | OpenAPI 3 contract for this API (JSON) |
| `GET` | `/docs` | Swagger UI for the API (loads UI assets from a CDN) |

### Interactive API testing

Every node describes its own API and serves a Swagger UI page, so you can exercise
the endpoints from a browser instead of hand-writing `curl`:

```bash
go run ./backend/cmd/kvnode
# then open http://localhost:8080/docs and try PUT/GET/DELETE and /v1/status
```

The raw contract is at `/v1/openapi.json` — it works offline and in any tool
(Postman, Redoc, the Swagger editor). `/docs` loads the Swagger UI bundle from a
CDN, so that page itself needs network access. A Go test
(`backend/internal/httpapi/openapi_test.go`) fails if the spec and the registered routes
diverge, so the document cannot silently rot.

## Dashboard

Each node serves a small web dashboard at `/` (same origin as the API). It shows
every cluster node's role, term, leader, and commit/applied indices, and lets you
exercise `PUT`/`GET`/`DELETE` against the leader. The browser reaches peers
through a server-side `/node/{id}/` proxy, so nodes only need to be reachable
from each other — not from the browser.

The dashboard has two tabs: **Cluster** (the view above) and **API**, a small
playground where you pick a method, enter a path, and see the raw response —
including the status and the `X-Raft-Leader` header. Unlike the console, it does
not retry a `503`, so a non-leader answer is visible as-is.

The playground is driven by the node's own contract: it reads
`GET /v1/openapi.json` and derives its endpoint presets, the methods allowed per
path, and the declared status codes it shows. If the spec cannot be fetched it
falls back to a built-in endpoint list and keeps working.

The dashboard's TypeScript types come from the same document, so the frontend
cannot drift from the handlers: `npm run codegen` (`make web-codegen`) turns
`backend/internal/httpapi/openapi.json` into
`frontend/src/api-types.gen.ts`, and `npm run build` runs it first.

Build it once; the node embeds the output:

```bash
make web-build    # vite build -> backend/internal/webui/dist (embedded via go:embed)
go run ./backend/cmd/kvnode -id node1 -http :8080
```

Without a build, the binary serves a committed placeholder page, so
`go build ./...` keeps working with no Node installed.

## Run a single node

```bash
go run ./backend/cmd/kvnode -id node1 -http :8080

# in another shell
curl -i -X PUT --data-binary 'hello' http://localhost:8080/v1/kv/greeting
curl -i http://localhost:8080/v1/kv/greeting
curl -i -X DELETE http://localhost:8080/v1/kv/greeting
curl -i http://localhost:8080/v1/status
```

## Run a 3-node cluster

Two ways, same topology: three nodes, one keyspace, majority commit.

**Local processes (no docker)** — the Done-when check, fully scripted:

```bash
scripts/cluster-smoke.sh   # or: make cluster-smoke
```

It starts three nodes on `:9081`-`:9083` with persistent data dirs, waits for a
leader, writes a key, kills the leader, waits for a new leader, and reads the
key back — proving committed state survives a leader failure.

**Docker Compose** — the containerized cluster ([`deploy/`](deploy)):

```bash
make cluster-up     # docker compose -f deploy/docker-compose.yml up -d --build
curl -s localhost:8081/v1/status   # node1 (also 8082, 8083)
make cluster-down
```

Peers are addressed by service name (`node2=http://node2:8080`) and each node
persists its log in a named volume, so a restart recovers it. Writes must reach
the current leader; a follower answers `503` with `X-Raft-Leader`. Kill the
leader container (`docker compose -f deploy/docker-compose.yml kill node1`) and a
survivor takes over.

## Design choices

- **No sharding** — one keyspace, replicated to every node (3-node quorum).
- **The log is the write-ahead log** — every change goes through the Raft log;
  applying committed entries updates an in-memory map.
- **Leader reads (MVP)** — reads go to the leader to avoid stale reads early on.
- **Quorum loss stops writes** — a leader that cannot reach a majority rejects
  writes rather than best-effort accepting them.
- **Automatic snapshots** — once a configurable number of entries (1000 by
  default) has been applied past the last snapshot, the state machine is
  snapshotted and the log prefix dropped, so the log does not grow without
  bound and restarts replay only the tail.
- **Snapshot catch-up** — a follower that falls behind the compacted log prefix
  is caught up with the leader's snapshot via the `InstallSnapshot` RPC instead
  of the missing entries.

## Build order

1. Single-node: HTTP server + in-memory map, fully tested
2. Leader election: roles, timeouts, majority
3. Log replication: append entries, commit, apply
4. Persistence: log to disk + recovery on restart
5. Failure tests: kill the leader, confirm a new one is elected and data survives

## Done when

You can run three nodes locally, write a key, kill the leader, and still read the
committed value from the newly elected leader. `make cluster-smoke` checks this
end to end.

## Development

The docker compose **dev sandbox** gives you the full toolchain (Go, Node,
`gh`, `air` fast reload) with your host GitHub config mounted read-only and
`GH_TOKEN`/`GITHUB_TOKEN` forwarded at runtime — never baked into the image.

```bash
# Copy .env.example to .env to override ports/tokens if needed
cp .env.example .env

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
make check           # all of the above (backend gate)

cd frontend
npm ci               # install frontend deps
npm run dev          # Vite dev server (proxies to a running node)
npm run codegen      # regenerate src/api-types.gen.ts from openapi.json
npm test             # frontend unit tests
make web-check       # frontend gate: install + lint + format:check + test + build
```

## Testing

### Backend

Bring up a node (dev sandbox with `air` hot reload):

```bash
make dev-up        # kvnode on :8080 (set HOST_PORT to override)
make dev-log       # follow logs
make dev-down      # stop when done
```

**Manual testing (OpenAPI/Swagger):**

- **Swagger UI:** http://localhost:8080/docs — Try it out against the live node.
  (The UI loads from a CDN, so the browser needs network.)
- **Raw spec:** http://localhost:8080/v1/openapi.json — import into Postman,
  Redoc, the Swagger editor, or for client generation.
- **Dashboard API playground:** http://localhost:8080/ → **API** tab — spec-driven
  presets, per-path methods, and declared status codes; does not retry 503s so
  `X-Raft-Leader` is visible (see [Interactive API testing](#interactive-api-testing)).
- **curl spot checks:**
  ```bash
  curl -X PUT --data-binary 'hello' http://localhost:8080/v1/kv/demo
  curl http://localhost:8080/v1/kv/demo
  curl -X DELETE http://localhost:8080/v1/kv/demo
  curl http://localhost:8080/v1/status
  ```

**Automated testing:**

```bash
go test ./backend/internal/httpapi/ -v    # contract tests: drive requests from the OpenAPI spec; fail on spec↔handler drift
go test ./...                             # all unit tests
make check                                # full backend gate: gofmt -l, go vet, go test, go build
```

### Frontend

**Automated testing:**

```bash
cd frontend && npm ci
npm test                # vitest + Testing Library + user-event (jsdom)
npm run test:watch      # watch mode while iterating
npm run lint
npm run format:check
make web-check          # frontend gate: install + lint + format:check + test + build
```

**Manual testing (browser):**

- **Dev server (hot reload):** With the backend node already up (`make dev-up`),
  run `make web-dev` and open http://localhost:5173. Vite proxies `/v1`, `/raft`,
  `/cluster`, `/node`, and `/docs` to `http://localhost:8080` (override with
  `VITE_BACKEND=<node-url>`).
- **Production-like (embedded SPA):** Run `make web-build` (tsc + vite build;
  regenerates TypeScript types from the OpenAPI spec first) to build into
  `backend/internal/webui/dist`; the running `kvnode` (or `air` on rebuild) serves
  the dashboard from the same origin at http://localhost:8080.

### End-to-end

```bash
make cluster-smoke      # 3-node failure check: write a key, kill the leader, read it from the new leader (Go + curl only)
```

The project follows an issue-first, branch-based workflow — see
[`WORKFLOW_PROMPT.md`](WORKFLOW_PROMPT.md) and [`AGENTS.md`](AGENTS.md).
