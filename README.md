# StratumMC

> A Project/Room-centered collaborative Minecraft technical testing control plane for invited advanced players — TMC, redstone, and world-mechanics researchers.

<!-- README-I18N:START -->

**English** | [汉语](./README.zh.md)

<!-- README-I18N:END -->

StratumMC is **not** a generic Minecraft hosting panel. It coordinates shared testing rooms, temporary fork sessions, semantic checkpoints, and approved artifact management under explicit resource limits — designed for reproducible technical experimentation, not unlimited per-user sandboxes.

---

## Why StratumMC

Traditional server panels treat each Minecraft instance as the primary object. StratumMC inverts that: the **collaboration unit** (Project → Room → Session) is the product, and the running server is just one execution target underneath.

The default workflow matches how technical research actually happens:

```text
Join a Project
  → enter a shared Room
  → collaborate with others on a long-lived session
  → branch a temporary Fork Session for a risky experiment
  → save a semantic Checkpoint when something works
  → optionally promote that checkpoint to a project milestone
```

Everything is scriptable from the CLI first. A future Web UI is a convenience layer, not the primary interface.

---

## Features

**Domain model** — Project, Room, Session, Checkpoint, Artifact, Environment, ResourcePolicy, and Operation with explicit state transitions, audit history, and durable coordination.

**Multi-Agent runtime supervision** — A Controller is the source of truth for metadata and scheduling; Agents own process lifecycle, runtime directories, and resource observation. Agents auto-register and heartbeat every 30s.

**Declarative RuntimeProfiles** — Launch behavior is described in JSON (`runtime-profiles/*.json`), never via shell commands or user-supplied executables. Hot-reload via file watcher.

**Three bundled environments** — 1.12 Forge (Java 8), 1.17 Fabric + MCDR + Carpet (Java 17), and Latest Fabric (auto-resolved from Mojang manifest, Java 21).

**Lucy integration** — Embedded Go library (`github.com/mclucy/lucy`) handles manifest generation, dependency resolution, lock files, package installation, and integrity verification. Never used as a runtime owner.

**MCDR supervision** — MCDR runs as a child RuntimeProfile under Agent ownership: start/stop/restart, stdin command injection, log-pattern readiness checks, graceful stop, crash detection, config.yml generation, and Python venv bootstrap.

**Server jar provisioning** — Vanilla (Mojang), Fabric (Fabric Maven), Paper (Paper API), and Forge 1.12.2 (Forge Maven) with proxy support.

**World checkpointing** — Four consistency levels (`metadata_only`, `stopped`, `best_effort`, `command_quiesced`), zip + SHA-256 snapshots, zip-slip-protected restore, world profile merge, and pre-operation checkpoints before restart and artifact apply.

**Artifact workflow** — Upload, hash (SHA-256), approve, stage, materialize, apply, verify. Unapproved artifacts never attach to shared sessions.

**Resource-aware scheduling** — Global, per-project, and per-user limits with queueing and denial reasons.

**Container orchestration** — `Dockerfile.agent` parameterized by Java version, `docker-compose.yml` with three isolated agents (Java 8/17/21) connecting to a host Controller.

---

## Architecture

```text
┌──────────────────────────┐        ┌─────────────────────────────┐
│      Controller          │        │           Agent(s)          │
│  (source of truth)       │        │   (owns runtime lifecycle)  │
│                          │◄──────►│                             │
│  • Projects / Rooms      │ HTTP   │  • Process supervision      │
│  • Sessions metadata     │  +     │  • RuntimeProfile registry  │
│  • Checkpoints metadata  │ tokens │  • Lucy materialization     │
│  • Artifacts metadata    │        │  • MCDR / Java / server jar │
│  • Scheduling / audit    │        │  • World snapshot / restore │
└──────────────────────────┘        └─────────────────────────────┘
              │                                   │
              ▼                                   ▼
       CLI (`stratum`)                Runtime directories
                                      Artifacts / mods / worlds
```

- **Controller** (`cmd/stratum-controller`) — authoritative metadata, scheduling, agent registry, audit history
- **Agent** (`cmd/stratum-agent`) — process lifecycle, RuntimeProfile execution, materialization, checkpoint workers
- **CLI** (`cmd/stratum`) — primary user interface, scriptable, no frameworks beyond cobra
- **Lucy CLI** (`cmd/lucy`) — standalone wrapper around the embedded Lucy library

See [`docs/architecture.md`](docs/architecture.md) for full boundaries and ownership rules.

> [!NOTE]
> MCDR is supervised by the Agent as a child RuntimeProfile — it never owns the top-level Stratum lifecycle. Lucy is a planning/resolution library, never a runtime owner. Uploaded jars are quarantined by approval workflow and never affect base worlds.

---

## Quick Start

### Prerequisites

- Go 1.25+
- Java 8 (for 1.12), Java 17 (for 1.17), Java 21 (for latest) — only needed on the Agent host that runs each environment
- Python 3.9+ — only needed on Agents that run MCDR-backed profiles
- [Task](https://taskfile.dev) (`go install github.com/go-task/task/v3/cmd/task@latest`) — optional but convenient

### 1. Clone

```bash
git clone --recurse-submodules https://github.com/stratummc/stratum.git
cd stratum
# If already cloned without submodules:
git submodule update --init --recursive
```

### 2. Build

```bash
task build           # binaries land in dist/local/
# Or directly:
go build -o dist/local/stratum ./cmd/stratum
go build -o dist/local/stratum-agent ./cmd/stratum-agent
go build -o dist/local/stratum-controller ./cmd/stratum-controller
```

### 3. Run the test suite

```bash
go test -count=1 ./...
```

### 4. Start a Controller and an Agent

```bash
# Terminal 1 — Controller
go run ./cmd/stratum-controller serve --listen 127.0.0.1:8080 --data-dir .stratum/data

# Terminal 2 — Agent (auto-registers with the Controller)
go run ./cmd/stratum-agent serve \
  --listen 127.0.0.1:8787 \
  --controller-url http://127.0.0.1:8080 \
  --runtime-root .stratum/runtime
```

### 5. Create a project, room, and session

```bash
STRATUM="go run ./cmd/stratum --data-dir .stratum/data"

$STRATUM projects create --id demo --name "Demo Project"
$STRATUM rooms create --id lab --project demo --name "Lab Room"
$STRATUM sessions create --id sess-1 --project demo --room lab
```

### 6. Launch the session under an Agent

```bash
$STRATUM --agent-url http://127.0.0.1:8787 sessions start \
  --id sess-1 --actor researcher --runtime-profile mcdr-fabric-1.17

$STRATUM --agent-url http://127.0.0.1:8787 sessions inspect --id sess-1
$STRATUM --agent-url http://127.0.0.1:8787 sessions logs --id sess-1
$STRATUM --agent-url http://127.0.0.1:8787 sessions stop --id sess-1
```

Discover available RuntimeProfiles with `stratum agents runtime-profiles --id <agent-id>`.

### Docker Compose (three isolated Agents)

```bash
cp .env.example .env   # adjust if needed
docker compose up -d   # starts agent-java8 / java17 / java21
```

Each container auto-registers with the host Controller at `host.docker.internal:8080`.

---

## Workflows

A typical technical-research cycle:

```bash
# 1. Set up a shared room
stratum rooms create --id 117-main-lab --project gtmc --name "1.17 Main Lab"

# 2. Branch a fork session for a risky experiment
stratum sessions create --id fork-rng-test --project gtmc --room 117-main-lab
stratum sessions start --id fork-rng-test --runtime-profile mcdr-fabric-1.17

# 3. When something works, checkpoint it
stratum checkpoints create \
  --id cp-working-piston-array \
  --session fork-rng-test \
  --actor alice \
  --consistency-level command_quiesced \
  --capture-world-profile \
  --notes "Stable piston array layout before scaling"

# 4. Upload a mod artifact for the project
stratum artifacts upload \
  --id carpet-fixes-1.0.jar \
  --project gtmc --file ./carpet-fixes-1.0.jar \
  --actor alice

# 5. Restore to the checkpoint later
stratum checkpoints restore --id cp-working-piston-array --session fork-rng-test --auto-stop --auto-start
```

See [`docs/workflows/`](docs/workflows/) for end-to-end examples covering experiment branching, configuration rollback, and reproducible testing.

---

## Documentation

- [`docs/architecture.md`](docs/architecture.md) — component boundaries, ownership rules, domain model
- [`docs/runtime.md`](docs/runtime.md) — Agent runtime supervision and RuntimeProfile system
- [`docs/cli-reference.md`](docs/cli-reference.md) — complete CLI command reference
- [`docs/checkpoints.md`](docs/checkpoints.md) — checkpoint creation, consistency levels, and rollback
- [`docs/world-profile.md`](docs/world-profile.md) — world configuration capture and restore
- [`docs/operations.md`](docs/operations.md) — durable operations, idempotency, audit
- [`docs/storage.md`](docs/storage.md) — repository abstractions and metadata durability
- [`docs/lucy-integration.md`](docs/lucy-integration.md) — Lucy dependency management
- [`docs/mcdr.md`](docs/mcdr.md) — MCDR child RuntimeProfile contract
- [`docs/agent.md`](docs/agent.md) — Agent HTTP transport and capabilities
- [`docs/security.md`](docs/security.md) — safety rules and artifact isolation
- [`docs/status.md`](docs/status.md) — current implementation status
- [`docs/routemap.md`](docs/routemap.md) — phased development roadmap
- [`docs/mvp.md`](docs/mvp.md) — MVP scope and explicit non-goals
- [`MINECRAFT_LAUNCH.md`](MINECRAFT_LAUNCH.md) — what is and isn't proven about real Minecraft boot

JSON schemas for every persisted object live in [`schemas/`](schemas/).

---

## Safety Boundaries

- Base worlds are immutable / read-only.
- Shared rooms enforce stricter permissions than fork or private sessions.
- Dangerous operations (restart, artifact apply) create a pre-operation checkpoint first.
- Uploaded jars require metadata, hash, and approval before touching a shared session.
- RuntimeProfiles are declarative JSON only — the CLI never accepts executable or shell-command input.
- World restore uses zip-slip protection (symlinks, `..`, absolute paths rejected).
- Agent containers run with isolated named volumes.

---

## Status

Phases 1–4 (core infrastructure, runtime execution, world management, multi-environment support) and Phases 6a/6c (multi-agent coordination, container orchestration) are complete. Real Minecraft boot via MCDR is the current focus — existing tests verify Stratum's runtime plumbing; a true Java/Minecraft smoke test is the next milestone.

See [`docs/status.md`](docs/status.md) for the authoritative, dated status table.

> [!IMPORTANT]
> Authentication is currently shared-token only. User accounts, RBAC, and project membership (Phase 6b) are not yet implemented.

---

## Development

```bash
task fmt     # go fmt
task vet     # go vet
task test    # go test -count=1 ./...
task ci      # deps + vet + test + linux-amd64 build (mirrors GitHub Actions)
```

Before committing, run `gofmt` on changed Go files and `go test ./...`. Follow the atomic change and commit-message prefix policy (`docs:`, `domain:`, `lifecycle:`, `agent:`, `cli:`, `test:`, …) documented in [`docs/workflow.md`](docs/workflow.md) and [`AGENTS.md`](AGENTS.md).

---

## License

Apache 2.0 — see [`LICENSE`](LICENSE).
