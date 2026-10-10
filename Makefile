# distributed-key-value-store — local development gate.
#
# CI runs only on `dev` -> `main` PRs, so `make check` is the gate that
# feature branches rely on. Run it before every push.

GO ?= go

.PHONY: check fmt vet test build run clean

## check: format + vet + test + build (the local gate before pushing)
check: fmt vet test build

fmt:
	@echo "==> gofmt"
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

vet:
	@echo "==> go vet"
	$(GO) vet ./...

test:
	@echo "==> go test"
	$(GO) test ./...

build:
	@echo "==> go build"
	$(GO) build ./...

run:
	$(GO) run ./cmd/kvnode

clean:
	@echo "cleaning"
	rm -f ./kvnode
