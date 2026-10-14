# AGENTS.md

> **Source**: Product sections below are populated from `PROJECT_CONTEXT.md`.

### Product

## Project Objective

distributed-key-value-store — a minimal, educational distributed key-value store built from scratch with a hand-rolled Raft consensus algorithm.

### Problem

Most single-server key-value stores lose availability when the server fails. We want a simple replicated store where multiple nodes agree on the write order, so reads and writes stay consistent (and available) when a node fails — for learning purposes.

### Goal

Users should be able to run a small cluster (3 nodes by default) and perform key-value operations (PUT/GET/DELETE) against it. The MVP starts single-node, then adds leader election, log replication, and persistence with Raft, so that a value written before a leader failure remains readable after a new leader is elected.

### Main Technology

- Language: Go (standard toolchain)
- Consensus: hand-rolled Raft (educational — not production-ready)
- Persistence: Raft log on disk; automatic snapshots + log compaction
- HTTP API: `net/http` (`PUT/GET/DELETE /v1/kv/{key}`, `GET /v1/status`), with an OpenAPI contract at `GET /v1/openapi.json` and Swagger UI at `GET /docs`
- Dashboard: React + Vite + TypeScript (`frontend/`), embedded via `backend/internal/webui`
- Orchestration: Docker Compose (3-node local cluster)
- Testing: `go test` (standard library `testing`); Vitest for the dashboard

### Core Architecture

```text
Client (curl / HTTP client)
  ↓
HTTP API (PUT/GET/DELETE /v1/kv/{key}, GET /v1/status)
  ↓
State machine (in-memory KV map, applied from committed log entries)
  ↓
Raft (leader election, log replication, commit/index, heartbeats)
  ↓
Persistent log (disk) + snapshots (compacted log)
  ↕
Peer-to-peer RPC between nodes
```

## Project Structure

The MVP is built in order (see Build Order in `PROJECT_CONTEXT.md`): single-node
KV, then Raft roles, replication, persistence, and finally the operator
dashboard. The tree:

```
distributed-key-value-store/
├── backend/
│   ├── cmd/kvnode/      # node entry point (flags, wiring, startup)
│   └── internal/
│       ├── httpapi/     # HTTP handlers: /v1/kv/{key}, /v1/status, /v1/openapi.json
│       ├── raft/        # hand-rolled Raft (roles, elections, replication, storage)
│       ├── kv/          # in-memory state machine
│       └── webui/       # dashboard: embedded SPA, /docs (Swagger UI), /cluster, /node/{id}/ proxy
├── frontend/            # React + Vite + TS dashboard source
├── deploy/              # 3-node cluster: Dockerfile + docker-compose.yml
├── scripts/             # dev-sandbox + cluster-smoke helper scripts
├── Dockerfile           # dev sandbox image (Go + gh + air + node)
├── docker-compose.yml   # single `dev` service for local development
├── go.mod  go.sum       # Go module root stays at the repo root
├── mise.toml            # toolchain pins (go, gh, node) — host + dev image
├── .air.toml            # air fast-reload config (runs ./backend/cmd/kvnode)
├── Makefile
├── opencode.json
└── .github/workflows/ci.yml
```

The dashboard is a Vite + React + TypeScript app in `frontend/`. `make web-build`
compiles it to `backend/internal/webui/dist`, which the node embeds via
`go:embed`. The committed `backend/internal/webui/dist/placeholder.html` keeps a
plain `go build` working without Node; the built assets are gitignored.

`backend/internal/httpapi/openapi.json` is the source of truth for the HTTP API:
Go contract tests keep it in sync with the handlers, and `npm run codegen`
(`make web-codegen`) regenerates `frontend/src/api-types.gen.ts` from it.
`npm run build` runs codegen first, so the committed types cannot go stale.

## Development Commands

```bash
go build ./...           # build all packages
go vet ./...             # static analysis
go test ./...            # run tests
go test -tags=integration ./...  # opt-in multi-node failure tests
gofmt -l .               # list unformatted files (empty = clean)
go run ./backend/cmd/kvnode      # run a node

cd frontend              # frontend
npm ci                   # install from the lockfile
npm run dev              # Vite dev server (proxies to a local node)
npm run lint             # oxlint
npm run format:check     # oxfmt (use `npm run format` to fix)
npm test                 # vitest
npm run codegen          # regenerate src/api-types.gen.ts from openapi.json
npm run build            # type-check + build into backend/internal/webui/dist
```

### Run All Checks

```bash
make check        # Go gate: gofmt -l, go vet, go test, go build
make web-check    # frontend gate: npm ci, lint, format:check, test, build
```

`make check` runs the same Go gate CI enforces; `make web-check` runs the
frontend gate. Prefer them over listing commands separately: agents follow
whatever a target runs, so a check missing from it is a check nobody performs.
Add new checks to the `Makefile`.

Multi-node failure tests are opt-in behind the `integration` build tag and are
**not** part of `make check`. Run them with `make test-integration` (i.e.
`go test -tags=integration ./...`).

The frontend is optional to build: the node embeds
`backend/internal/webui/dist/placeholder.html` (committed), so a plain `go build ./...`
works without Node. Run `make web-build` to embed the real dashboard.

### 3-node cluster

`make cluster-smoke` runs three local processes (`scripts/cluster-smoke.sh`,
Go + curl only) and checks the Done-when: write a key, kill the leader, read the
committed value from the new leader — then tears them down. For a persistent,
browsable cluster (no docker), use `make cluster-local-up` /
`cluster-local-status` / `cluster-local-down` (`scripts/cluster-local.sh`): it
starts three nodes on `127.0.0.1:9081`-`:9083` and leaves them running.
`make cluster-up` / `cluster-down` manage
the containerized cluster in `deploy/docker-compose.yml` (requires docker). The
docker path cannot be exercised from this environment — no docker socket.

### Dev sandbox

The docker compose `dev` service is the development environment (gh + opencode
pre-configured, `air` fast reload, no docker socket). Host-only tools are
bootstrapped with `./scripts/setup-host-tools.sh` (installs mise, authenticates
gh). Key targets:

```bash
make dev-up        # build + start the sandbox (requires docker on the host)
make dev-restart   # restart the dev container after Dockerfile/deps changes
make dev-shell     # shell inside the container (alias: make shell)
make dev-opencode  # run opencode inside the container (alias: make opencode)
make dev-log       # follow container logs
make dev-down      # stop and remove the sandbox
```

`make dev-up` forwards `GH_TOKEN` or `GITHUB_TOKEN` (whichever is valid) into
the container at runtime — values stay in the host environment, never in the
image. Host `~/.config/gh`, `~/.config/opencode`, `~/.gitconfig`, and the
`opencode` binary are mounted read-only. Go module/build caches live under
`/workspace/.go`; opencode/gh runtime state under `/workspace/.xdg-*` (both
gitignored).

## Runtime Environment

The AI agent (opencode) runs at the repository root. For development, a docker
compose **dev sandbox** provides the full toolchain (Go, `gh`, `air` fast
reload) with host GitHub config mounted read-only — see Development Commands.
The in-container agent has **no docker socket** (by design): it cannot manage
containers, so the three-node cluster compose remains a later milestone.

Install Go, GitHub CLI, and OpenCode as needed for your environment; on a fresh
host, `./scripts/setup-host-tools.sh` bootstraps mise + gh.

### What the agent CAN do

- Run the Go toolchain (`go build`, `go test`, `go vet`, `gofmt`, `go run`)
- Run git and GitHub CLI commands
- Start local node processes
- Start/use the dev sandbox from the host: `make dev-up`, `make dev-shell`, `make dev-opencode`
- Run every script under `scripts/` (entrypoint, token resolution, sandbox bring-up)

### What the agent CANNOT do

- Run `docker` or `docker compose` commands in this environment (no docker on the host here)
- Access the Docker socket — including inside the dev sandbox, which mounts none by design
- Modify the host filesystem beyond the workspace

## Environment Variables

- `GH_TOKEN` / `GITHUB_TOKEN` — GitHub credentials, read from the host and
  forwarded into the dev sandbox at runtime. Either name works; neither is
  defined here, and values are never baked into the image.
- `HOST_HOME` — optional override for the host home directory used by the
  read-only config mounts (defaults to `$HOME`).
- This project reads node configuration from command-line flags (node id,
  listen address, peer list) rather than a secret environment file. If a `.env`
  or `.env.*` file is introduced later, it is gitignored and the secret-file
  rules below apply.

## Key Conventions

- **Assignee**: `fiwon123` — `workflow.md` tells agents to read `ASSIGNEE` from here before assigning issues and PRs
- **GitHub credentials**: `GH_TOKEN` or `GITHUB_TOKEN` — either name works. Do not set either here: values belong in the host environment, and the dev sandbox forwards whichever is valid at runtime.
- **Dev sandbox**: docker compose `dev` service (gh + opencode pre-configured, no docker socket); `make dev-up` / `dev-restart` / `dev-shell` / `dev-opencode` / `dev-down`
- **Model assignments**: an example, not a recommendation — `opencode.json` assigns `opencode/big-pickle` to `build` and `opencode/mimo-v2.6-flash-free` to `plan`. Change them to whatever your install offers; no prompt depends on which model an agent runs. Verify an id with `opencode models` first.
- Linting / formatting: `gofmt` for all Go code; `go vet` for static analysis
- Testing: the standard `testing` package (`go test ./...`) for all packages
- CI: GitHub Actions runs formatting + vet + test + build on `dev` → `main` PRs only — never on feature branch PRs or push to `dev`
- Local testing: run `make check` before pushing feature branches
- Educational scope: prefer clear, readable Raft over clever optimizations; the project is not production-ready

## Environment and Secret-File Rules

- Never open, read, print, summarize, quote, or send the contents of `.env`, `.env.*`, `.envrc`, or any other environment/secret file unless the user explicitly gives permission in the current conversation. `.env.example` is the one exception: it holds placeholders and is safe to read.
- Never run commands that reveal environment values — `printenv`, `env`, and the Docker equivalents.
- Never include secret values in the final answer. Refer to them only by variable name.
- If the user explicitly permits reading environment configuration, read only explicitly approved non-secret variables and check existence rather than printing values.

### Enforcement (backstop, not the permission)

`opencode.json` denies reading, editing, and bash access to `.env*` paths, and
denies commands that dump the environment. It matches **command strings and file
paths**, not intent, so it has known gaps: an allowed interpreter can still read a
file, and a repo-wide `grep` ignores `.gitignore`. The `.gitignore` entries for
`.env` and `.envrc` are what protect the Grep tool — keep them.

## Development Workflow

### Git Workflow

- Branch from `dev` (not `main`)
- Never push directly to `dev` or `main`
- Branch naming: `<type>/<issue-number>-<slug>` (e.g., `feat/12-single-node-kv`)
- Branch types: `feat/`, `fix/`, `refactor/`, `docs/`, `test/`, `chore/`, `ci/`
- Use conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`, `ci:`
- All testing is local for feature branches — CI only runs on `dev` → `main` PRs
- Do NOT merge pull requests unless explicitly instructed
- Do NOT automatically create release PRs or merge to main — user must explicitly request
- Always return to `dev` branch after completing any merge
- Every change is tracked on GitHub: **issue → branch → PR → merge**

### Linking

- Every branch includes the issue number: `feat/12-single-node-kv`
- Every PR references its issue: `Closes #<number>`
- Every issue is assigned to a milestone before work begins
- Every PR is assigned to a milestone when created
- No orphaned branches, PRs, or issues

### Branch Strategy

```text
main          ← release merges (CI runs here — dev→main PRs only)
  └── dev     ← integration branch (all feature branches merge here)
        ├── feat/12-single-node-kv
        ├── fix/17-leader-election-timeout
        └── refactor/31-log-persistence
```

- Feature branches: `<type>/<issue>-<slug>` → merge to `dev`
- Release: `dev` → `main` (PR triggers CI — user must explicitly request)
- Hotfixes: `fix/<issue>-<slug>` → merge to `dev`, cherry-pick to `main` if urgent

### Milestones

- Every issue MUST be assigned to a milestone before work begins
- Milestones represent releases or sprint iterations
- Use `gh issue edit <number> --milestone "<milestone-name>"`
- Track milestone progress on the GitHub Milestones page

### Agents

Agent definitions live in `opencode.json` and `.opencode/agents/`:

- **build** (primary): Full development work with all tools enabled
- **plan** (primary): Analysis and planning without making changes — restricted to read-only subagents (explore, reviewer only)
- **reviewer** (subagent): Reviews code for bugs, security, and regressions
- **backend** (subagent): Implements the Go API, Raft, and state machine — edit scoped to `backend/cmd/**` and `backend/internal/**`
- **frontend** (subagent): Implements the dashboard in `frontend/` and its Go embedding — edit scoped to `frontend/**` and `backend/internal/webui/**`
- **tester** (subagent): Writes and runs Go tests — edit scoped to `*_test.go` and `testdata/`

The `visual` agent from the engineering-workflow templates is deliberately not
defined here — there is no design surface to own beyond the one dashboard, so a
subagent whose edit scope points at a directory that does not exist is an
expensive way to say nothing.

Each agent's instructions live in exactly one place: the body of
`.opencode/agents/<name>.md`. Model, mode, and permissions live in
`opencode.json`. Do not add a `prompt` key to `opencode.json` — it creates a
second prompt source that drifts from the markdown.

Subagent permissions must match what the markdown tells the agent to do. If
`plan.md` lists an agent but `task` denies it, the agent is instructed to use a
tool it cannot reach.

### Skills

- Load relevant skills when working on specific domains
- `github-workflow` (`SKILL.md`) is the git/issue/PR command cookbook; `workflow.md` remains the policy authority.

### Session Memory (3-tier)

- At the START of every chat: read `.opencode/memory/global.md` then `.opencode/memory/current.md` and resume from "Next action".
- Update `global.md` at meaningful milestones (issue created, PR opened, merged, decision made); update `current.md` after each step and at session end; rotate the oldest archive so only 3 remain.
- Keep sizes bounded: `global.md` ≤ ~200 lines, `current.md` ≤ ~100 lines, archive files ≤ ~50 lines each; never store secrets/tokens/credentials.
- The whole `.opencode/memory/` directory is gitignored and must never be committed. Full rules: `.opencode/instructions/memory.md`.

### Rules for Agents

- branch strategy and naming conventions;
- CI strategy (which PRs trigger CI, local testing requirements);
- milestone and issue conventions;
- development commands (test, format, vet, build);
- environment and secret-file rules;
- project-specific constraints (educational scope, quorum-loss behavior).

### CI triggers (read this before claiming a change is verified)

CI runs on release PRs (`dev` → `main`) and manual dispatch — never on a
feature-branch push. Two things can make a run verify nothing:

- a `paths:` filter that the change does not touch means the workflow does not
  trigger at all; and
- a job gated behind a label or `if:` condition comes back `skipped`, not `passed`.

**Skipped is not passed.** A triggered run whose jobs were all gated out, or a
run that never happened, is not coverage. Do not describe a change as "CI passed"
unless a run actually executed the job that covers it; if it did not, say the
change is unverified and test it locally.
