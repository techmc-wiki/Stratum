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

MCDR and Lucy are integrated:

- **Lucy** is embedded as a Go library (`github.com/mclucy/lucy`) for dependency resolution, package installation, and environment materialization
- **MCDR** may run as a child RuntimeProfile under Agent supervision for in-game command bridging (planning contract exists, executor pending)

See [docs/architecture.md](docs/architecture.md) for design boundaries and [docs/runtime.md](docs/runtime.md) for Agent ownership rules.

## Current Status

### ✅ Implemented

- **Core domain models** — Project, Room, Session, Checkpoint, Artifact, Environment, ResourcePolicy
- **Durable operations** — request correlation, idempotency, and audit history
- **Repository layer** — in-memory and filesystem-backed storage
- **HTTP Agent** — process lifecycle supervision with RuntimeProfile abstraction
- **Lucy integration** — embedded Go library for dependency resolution, manifest/lock files, and package installation
- **MCDR bridge** — planning-only contract for MCDR child RuntimeProfile integration
- **Environment materialization** — Lucy-driven package installation, artifact staging, and runtime workspace setup
- **RuntimeProfile registry** — declarative process launch configs with readiness checks and stop strategies
- **CLI** — standard-library implementation with no external frameworks

### ⚠️ Not Yet Implemented

- **Actual Minecraft server launching** — current RuntimeProfiles use test stubs; no real JVM/MCDR processes started
- **MCDR RuntimeProfile executor** — MCDR bridge defines launch plans but Agent does not yet execute them
- **Real Java runtime detection** — Java version validation stubs exist but no real JVM discovery
- **Server jar provisioning** — Fabric/Forge server download and verification not yet implemented
- **World checkpoint backup/restore** — checkpoint metadata exists but world file operations are stubs
- **1.12 or latest environments** — only 1.17 Fabric + MCDR + Carpet planned
- **Web UI** — CLI-only currently
- **Production orchestration** — no container/deployment tooling

### 🔐 Safety Boundaries

All tests use process stubs and do not execute user commands, launch shells, or start real JVM instances. Agent RuntimeProfile execution is isolated behind declarative JSON configs. The system is ready for controlled RuntimeProfile implementation.

## Quick Start

### 0. Clone with submodules

```bash
git clone --recurse-submodules https://github.com/stratummc/stratum.git
# Or if already cloned:
git submodule update --init --recursive
```

### 1. Build

```bash
# Install task runner if needed
go install github.com/go-task/task/v3/cmd/task@latest

# Build all components
task build

# Binaries will be in dist/local/
```

### 2. Run tests

```bash
go test ./...
```

### 3. Create a project and room

```bash
go run ./cmd/stratum --data-dir .stratum/data projects create --id demo --name "Demo Project"
go run ./cmd/stratum --data-dir .stratum/data rooms create --id demo-room --project demo --name "Demo Room"
go run ./cmd/stratum --data-dir .stratum/data sessions create --id demo-session --project demo --room demo-room
```

### 4. Start the HTTP Agent

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

- [docs/lucy-integration.md](docs/lucy-integration.md) — Lucy dependency management integration
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
