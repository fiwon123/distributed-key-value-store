## Testing Requirements

### General

- Tests are mandatory for new features and bug fixes
- Write tests before or alongside implementation
- Mock external services when possible — never call a real peer node or network service in a unit test
- Aim for critical-path coverage, not 100% line coverage

### Prerequisites / Services

- Unit tests must run with no external services — the state machine and Raft
  logic are tested in-process.
- Integration tests that spin up a multi-node cluster (Docker Compose or
  in-process peers) are opt-in: label them with a build tag (e.g.
  `//go:build integration`) or gate them behind an environment variable, and
  document the command that runs them in `AGENTS.md`.
- Prefer running unit tests in isolation; keep cluster tests clearly separated.

### Go

- **Framework**: the standard library `testing` package (`go test`)
- **Location**: `*_test.go` files next to the code they test
  (e.g. `internal/kv/store_test.go`)
- **Run**: `go test ./...`

#### Test Structure

```
internal/kv/
├── store.go
└── store_test.go        # tests for the package it sits in

cmd/kvnode/
├── main.go
└── main_test.go
```

#### Conventions

- Test functions: `func TestXxx(t *testing.T)`
- Prefer table-driven tests: a slice of cases with `name`, inputs, and expected output
- Use `t.Run(name, ...)` for subtests and `t.Helper()` in helpers
- Tests live in the same package (`package kv`) unless you need to test only the
  exported API, in which case use the `_test` suffix package
- Mock dependencies with small interfaces and fakes, not a mocking framework
- Use `t.TempDir()` for any on-disk state (e.g. the Raft log) so tests clean up after themselves

### Linting / Static checks

```bash
gofmt -l .      # lists files that are not gofmt-clean (empty output = clean)
go vet ./...    # static analysis
```

### Before Committing

1. `gofmt -l .` — must print nothing
2. `go vet ./...` — must pass
3. `go test ./...` — must pass
4. `go build ./...` — must build
5. Verify no regressions

`make check` runs steps 1–4 together.

### What "verified" means

A run that was skipped or never triggered is not a pass. CI for this project runs
only on `dev` → `main` PRs, so a feature branch has no CI signal — run `make check`
locally and say what you ran. Skipped is not passed.
