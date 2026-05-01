.PHONY: build run test test-db test-coordinator test-worker clean ts deps lint

BINDIR  := ./bin
SERVER  := $(BINDIR)/ctrq
CLI     := $(BINDIR)/ctrqctl

# ── TypeScript ──────────────────────────────────────────────────────────────
ts:
	npx tsc --project tsconfig.json

# ── Go build ────────────────────────────────────────────────────────────────
build: ts
	mkdir -p $(BINDIR)
	go build -o $(SERVER) ./cmd/ctrq
	go build -o $(CLI) ./cmd/ctrqctl

# ── Run server ───────────────────────────────────────────────────────────────
run: build
	$(SERVER)

# ── Tests ────────────────────────────────────────────────────────────────────
test:
	go test -race -count=1 ./...
	@command -v npx >/dev/null 2>&1 && npx vitest run || true

test-db:
	go test -race -v -count=1 ./internal/db/...

test-coordinator:
	go test -race -v -count=1 ./internal/coordinator/...

test-worker:
	go test -race -v -count=1 ./internal/worker/...

# ── Deps ─────────────────────────────────────────────────────────────────────
deps:
	go mod tidy
	npm install

# ── Lint ─────────────────────────────────────────────────────────────────────
lint:
	go vet ./...

# ── Clean ────────────────────────────────────────────────────────────────────
clean:
	rm -rf $(BINDIR)
	rm -f web/static/js/*.js
