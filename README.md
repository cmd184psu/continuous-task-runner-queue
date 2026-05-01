# ctrq — Continuous Task Runner Queue

A production-ready task queuing system with group-based concurrency pools, JWT authentication, SSE output streaming, and an embedded web UI.

## Features

- **Groups with pool limits** — each group has a configurable `pool_limit`; tasks run concurrently up to that limit
- **Group and task pause/resume** — stop scheduling at either the group or individual task level
- **Task types** — `exec`, `shell`, `script`, `migration`
- **Repeat scheduling** — tasks re-enqueue after a configurable cooldown
- **Sudo escalation** — per-task `sudo` flag
- **JWT authentication** — 5-digit passcode in `~/.ctrq.json`
- **SSE output streaming** — tail live stdout/stderr of any running execution
- **Metrics** — duration, scheduling delay, success/failure counts per task
- **Embedded web UI** — Obsidian dark-mode; optional, toggle via config
- **SQLite (no CGO)** — uses `modernc.org/sqlite`, no system libraries needed

## Prerequisites

- Go 1.23+
- Node.js 18+ and npm (for TypeScript compilation)

## Build

```bash
# Install all dependencies
make deps

# Build both binaries (compiles TS first, then Go)
make build

# Binaries are in ./bin/
./bin/ctrq       # server
./bin/ctrqctl    # CLI
```

## Configuration (`~/.ctrq.json`)

```json
{
  "port": 9898,
  "passcode": "12345",
  "ui_enabled": true,
  "db_path": "~/.ctrq.db",
  "groups": [
    { "name": "migrations", "pool_limit": 1, "allowed_types": ["migration", "shell"] },
    { "name": "builds",     "pool_limit": 5 },
    { "name": "batch",      "pool_limit": 10 }
  ]
}
```

| Field | Default | Description |
|---|---|---|
| `port` | `9898` | HTTP port |
| `passcode` | `"12345"` | 5-digit auth code |
| `ui_enabled` | `true` | Serve embedded web UI |
| `db_path` | `~/.ctrq.db` | SQLite database path |
| `groups` | `[]` | Seed groups on startup (DB is authoritative at runtime) |

Groups defined in the config are upserted into the database on startup. Additional groups can be created/edited/deleted via the API or UI at runtime.

## Running

```bash
./bin/ctrq
# ctrq listening on :9898  (ui=true)

# Custom config
./bin/ctrq --config /etc/ctrq.json
```

Open `http://localhost:9898` for the web UI. Enter your passcode to log in.

## CLI (`ctrqctl`)

Token is auto-fetched from `~/.ctrq.json` and cached in `~/.ctrq-token`.

```bash
# Server URL (default: from config port)
export CTRQ_URL=http://localhost:9898   # or use --url flag

# Health
ctrqctl health

# Groups
ctrqctl group list
ctrqctl group create --name workers --limit 4
ctrqctl group update workers --limit 8
ctrqctl group pause  migrations
ctrqctl group resume migrations
ctrqctl group delete empty-group

# Tasks
ctrqctl task list
ctrqctl task list --group batch

ctrqctl task add \
  --name my-backup \
  --group batch \
  --type shell \
  --args '{"shell":"tar -czf /backup/data.tar.gz /data"}' \
  --repeat \
  --cooldown 3600 \
  --priority 20

ctrqctl task add \
  --name run-migration \
  --group migrations \
  --type migration \
  --args '{"name":"0042_add_users"}'

ctrqctl task pause  my-backup
ctrqctl task resume my-backup
ctrqctl task enqueue my-backup     # force-run now (bypass cooldown)
ctrqctl task update my-backup --cooldown 7200
ctrqctl task delete my-backup

# Executions
ctrqctl executions
ctrqctl executions --task my-backup --limit 10

# Stream live output
ctrqctl output 42

# Metrics
ctrqctl metrics
ctrqctl metrics --group batch --hours 48
ctrqctl metrics --task my-backup
```

## Task Types

| type | args JSON | description |
|---|---|---|
| `shell` | `{"shell":"echo hello"}` | Run via `/bin/sh -c` |
| `exec` | `{"command":"rsync","args":["-av","src/","dst/"],"workdir":"/","env":{"KEY":"val"}}` | Direct exec |
| `script` | `{"path":"/opt/scripts/deploy.sh","args":["--env","prod"]}` | Execute script file |
| `migration` | `{"name":"0042_users","command":"migrate"}` | Run migration by name |

Add `"sudo": true` to any task to prepend `sudo`.

## API

All endpoints except `/api/auth/token` and `/api/health` require `Authorization: Bearer <token>`.

```
POST   /api/auth/token
GET    /api/health

GET    /api/groups
POST   /api/groups
GET    /api/groups/{name}
PUT    /api/groups/{name}
DELETE /api/groups/{name}
POST   /api/groups/{name}/pause
POST   /api/groups/{name}/resume

GET    /api/tasks[?group=X]
POST   /api/tasks
GET    /api/tasks/{name}
PUT    /api/tasks/{name}
DELETE /api/tasks/{name}
POST   /api/tasks/{name}/pause
POST   /api/tasks/{name}/resume
POST   /api/tasks/{name}/enqueue

GET    /api/executions[?task=X&limit=N]
GET    /api/executions/{id}/output     (SSE)

GET    /api/metrics[?group=X&task=X&hours=N]
```

### Authentication

```bash
TOKEN=$(curl -sX POST http://localhost:9898/api/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"passcode":"12345"}' | jq -r .token)

curl -H "Authorization: Bearer $TOKEN" http://localhost:9898/api/groups
```

### SSE output streaming

```bash
curl -H "Authorization: Bearer $TOKEN" \
     -H "Accept: text/event-stream" \
     http://localhost:9898/api/executions/42/output
```

## Testing

```bash
make test               # all Go tests + vitest
make test-db            # database layer only
make test-coordinator   # HTTP handler tests
make test-worker        # worker pool tests
```

## Architecture

```
cmd/ctrq/main.go         — single server binary: starts coordinator HTTP + worker goroutines
cmd/ctrqctl/main.go      — CLI binary

internal/
  config/    — load ~/.ctrq.json
  models/    — shared data types
  db/        — SQLite via modernc.org/sqlite (WAL mode, no CGO)
  worker/    — pool-aware scheduler + task executor + SSE output capture
  coordinator/ — chi HTTP router, JWT auth, REST handlers, SSE endpoint

web/
  web.go           — //go:embed static; http.FileServer
  static/index.html
  static/style.css — Obsidian dark mode CSS variables
  static/ts/       — TypeScript source (7 modules)
  static/js/       — compiled JS output (git-ignored except .gitkeep)
```

**Pool scheduling:** `GetEligibleTasks()` returns all eligible tasks from all groups. The worker loop queries `ListGroups()` for pool limits, counts running executions per group, and spawns goroutines up to `pool_limit - running`. Each goroutine captures task output into a ring buffer (last 1000 lines) served via SSE.

**SQLite concurrency:** WAL mode + `SetMaxOpenConns(1)` allows concurrent reads while serialising writes.

**Lock TTL:** Task locks expire after 10 minutes. For tasks expected to run longer, set a high `cooldown_seconds` value.

**Group pause:** Pausing a group prevents new tasks from starting. Currently-running tasks complete before the effect takes hold.

## Notes

- The passcode in `~/.ctrq.json` protects the API but is not a substitute for network-level security. For production, put ctrq behind a TLS reverse proxy.
- Metrics are retained indefinitely. Prune the `task_metrics` table periodically for long-running deployments.