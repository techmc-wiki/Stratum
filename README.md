# StratumMC

A Project/Room-centered collaborative Minecraft technical testing control plane for invited advanced players, especially TMC/redstone/world-mechanics researchers.

## What is StratumMC?

StratumMC coordinates:

- **Shared collaborative testing rooms** with long-lived server instances
- **Temporary fork sessions** for risky experiments that branch from rooms or checkpoints
- **Semantic checkpoints** that capture experiment snapshots with world state, environment, mods, and configs
- **Artifact management** with approval workflow for uploaded jars, datapacks, and configs
- **Resource-aware scheduling** to enforce global session limits and queueing
- **Agent-supervised runtimes** with process lifecycle ownership and clean separation from MCDR/Lucy

StratumMC is **not** a generic Minecraft hosting panel. It is designed for collaborative technical testing, not unlimited per-user sandboxes.

## Architecture

The control plane is split into three components:

- **Controller** — authoritative source of truth for projects, rooms, sessions, checkpoints, artifacts, permissions, and audit history
- **Agent** — owns runtime process lifecycle (start, stop, restart, logs, resource observation) and exposes runtime profiles
- **CLI** — `stratum` command-line tool for managing projects, rooms, sessions, and operations

MCDR and Lucy are optional future integrations:

- **MCDR** may run as a child RuntimeProfile under Agent supervision for in-game command bridging
- **Lucy** provides non-intrusive dependency manifests and lock files; it does **not** manage JVM processes or session scheduling

See [docs/architecture.md](docs/architecture.md) for design boundaries and [docs/runtime.md](docs/runtime.md) for Agent ownership rules.

## Current Status

This is an MVP skeleton. It implements:

- Core domain models (Project, Room, Session, Checkpoint, Artifact, Environment, ResourcePolicy)
- Durable operations with request correlation, idempotency, and audit history
- In-memory and filesystem-backed repositories
- HTTP Agent with a Go dummy RuntimeProfile for safe development
- Standard-library CLI with no external frameworks

It does **not** yet implement:

- Actual Minecraft server launching
- MCDR integration
- Lucy integration
- 1.12 or latest environments (only 1.17 Fabric + MCDR + Carpet stubs)
- Web UI
- Production container orchestration

The HTTP Agent supervises only the built-in Go dummy runtime. It does not launch Minecraft, MCDR, Lucy, shells, or user commands.

## Quick Start

### 1. Run tests

```bash
go test ./...
```

### 2. Create a project and room

```bash
go run ./cmd/stratum --data-dir .stratum/data projects create --id demo --name "Demo Project"
go run ./cmd/stratum --data-dir .stratum/data rooms create --id demo-room --project demo --name "Demo Room"
go run ./cmd/stratum --data-dir .stratum/data sessions create --id demo-session --project demo --room demo-room
```

### 3. Start the HTTP Agent

```powershell
# Terminal 1
go run ./cmd/stratum-agent serve --listen 127.0.0.1:8787

# Terminal 2
go run ./cmd/stratum --data-dir .stratum/process-test --agent-url http://127.0.0.1:8787 agents inspect --id local
go run ./cmd/stratum --data-dir .stratum/process-test --agent-url http://127.0.0.1:8787 sessions start --id demo-session --actor bryan
go run ./cmd/stratum --data-dir .stratum/process-test --agent-url http://127.0.0.1:8787 sessions inspect --id demo-session
go run ./cmd/stratum --data-dir .stratum/process-test --agent-url http://127.0.0.1:8787 sessions logs --id demo-session
```

The Agent exposes available RuntimeProfiles via `stratum agents runtime-profiles --id local`. Session start/restart accepts `--runtime-profile dummy-process`. The CLI never accepts executable or shell command input.

For shared-token authentication, add matching `--token` and `--agent-token` flags.

## Documentation

- [docs/architecture.md](docs/architecture.md) — component boundaries and ownership rules
- [docs/runtime.md](docs/runtime.md) — Agent runtime supervision and profiles
- [docs/operations.md](docs/operations.md) — durable operation lifecycle and correlation
- [docs/storage.md](docs/storage.md) — repository abstractions and metadata durability
- [docs/checkpoints.md](docs/checkpoints.md) — checkpoint creation and rollback semantics
- [docs/security.md](docs/security.md) — safety rules and artifact isolation
- [docs/mvp.md](docs/mvp.md) — MVP scope and non-goals
- [docs/workflow.md](docs/workflow.md) — development and review conventions

## Contributing

Follow the atomic change and commit policy in [docs/workflow.md](docs/workflow.md).

Run `gofmt` and `go test ./...` before committing.

See [AGENTS.md](AGENTS.md) for detailed project guidelines.
