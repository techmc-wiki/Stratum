# Implementation Status

Last updated: 2026-06-20

---

## ✅ Core Infrastructure

### Domain Models
- Project, Room, Session, Checkpoint, Artifact, Environment, ResourcePolicy
- Explicit state transitions and validation
- Metadata durability and audit history

### Storage
- Filesystem-backed repositories with atomic writes
- Content-addressed artifact blob storage with SHA-256 verification
- Append-only audit log

### Operations
- Durable operation coordination with idempotency
- Request correlation and timeout tracking
- Operation locking and conflict detection

### Controller
- HTTP API with agent registration/routing endpoints
- Agent registry with capability-based selection
- Resource-aware session scheduling
- Standard-library CLI (cobra-based, flag sub-commands in `cmd/stratum`)

### Agent
- HTTP transport for Controller-Agent communication
- Per-session runtime directory allocation
- RuntimeProfile registry with **hot-reload** (file watcher, 30s polling)
- Real OS process supervision (dummy, terminal, MCDR)
- Bounded log capture with circular buffer
- Process crash detection and graceful stop
- **Minecraft version polling** — `VersionCache` polls Mojang every 6h
- **Auto-registration** — `--controller-url` flag registers agent on startup, 30s heartbeat

---

## ✅ Lucy Integration

### Status: Production-ready as Go library

| Feature | Status |
|---------|--------|
| Manifest generation (`lucy.yaml`) | ✅ |
| Dependency resolution (`PlanEnvironment`) | ✅ |
| Lock generation (`lucy-lock.yaml`) | ✅ |
| Package installation (`InstallPackages`) | ✅ |
| Integrity verification (`VerifyIntegrity`) | ✅ |
| Status checking (`CheckStatus`) | ✅ |

---

## ✅ MCDR Integration

### Status: Fully implemented

| Feature | Status |
|---------|--------|
| MCDR process start/stop | ✅ |
| stdin command injection | ✅ |
| Log-pattern readiness check | ✅ |
| Graceful stop (stdin → force kill) | ✅ |
| Crash detection (exit code tracking) | ✅ |
| MCDR config.yml generation | ✅ |
| Python venv + MCDR install | ✅ |

---

## ✅ Java Runtime Detection

- `internal/agent/java/detector.go` — JVM discovery + version validation
- Java 8, 17, 21 all detected and validated
- Fallback handling when detection fails

---

## ✅ Server Jar Provisioning

| Core | Source | Status |
|------|--------|--------|
| Vanilla | Mojang manifest | ✅ |
| Fabric | Fabric Maven | ✅ |
| Paper | Paper API | ✅ |
| Forge 1.12.2 | Forge Maven (promotions JSON) | ✅ |
| **Latest** | Auto-resolved via Mojang manifest | ✅ |

---

## ✅ Phase 2 — Runtime Execution (Complete)

1. MCDR RuntimeProfile Executor v0
2. Proxy Configuration
3. Lucy Manifest Generation
4. Lucy Package Installation
5. Java Runtime Detection
6. Server Jar Provisioning

---

## ✅ Phase 3 — World Management (Complete)

### Checkpoint Consistency Levels

| Level | Behavior |
|-------|----------|
| `metadata_only` | Metadata and runtime status only |
| `stopped` | Stop → snapshot → restart |
| `best_effort` | `save-all flush` + snapshot |
| `command_quiesced` | `save-off` → flush → snapshot → `save-on` |

### Features
- World snapshot (zip + SHA-256) via `worldcheckpoint.Worker`
- World restore (unzip with zip-slip protection)
- CLI: create, list, inspect, restore, diff
- `--auto-stop` / `--auto-start` orchestration
- World Profile full/partial merge on restore

### Pre-Operation Checkpoints
- `sessions restart --pre-op-checkpoint`
- `artifacts apply execute --pre-op-checkpoint`
- KindPreOperation checkpoints with WorldStateRef

---

## ✅ Phase 4 — Additional Environments (Complete)

### 1.12 Forge
- `ServerForge` constant + schema validation
- Forge Maven download (promotions JSON → universal jar)
- `runtime-profiles/forge-1.12.json` — terminal runtime, Java 8
- `manifests/gtmc-1.12-forge-base.lucy.yaml` — JEI optional
- `docs/environments/gtmc-1.12-forge-base.json`

### Latest Fabric
- `ResolveLatestVersion()` hits Mojang manifest → current `26.2`
- `VersionCache` polls every 6h; used by Download + MaterializeEnvironment
- `runtime-profiles/fabric-latest.json` — MCDR + latest version
- `manifests/gtmc-latest-fabric-base.lucy.yaml` — Fabric API + Carpet
- `docs/environments/gtmc-latest-fabric-base.json`
- CLI: `environments latest-version`

### Three Environments Summary

| Environment | MC Version | Loader | Java | Profile |
|-------------|-----------|--------|------|---------|
| 1.17 Fabric | 1.17.1 | fabric | 17 | mcdr-fabric-1.17 |
| 1.12 Forge | 1.12.2 | forge | 8 | forge-1.12 |
| Latest | auto (26.2) | fabric | 21 | fabric-latest |

---

## ✅ Phase 6a — Multi-Agent Coordination (Complete)

### Agent Registry
- `internal/controller/agentregistry/` — Service and Repository
- `Agent{ID, Endpoint, Capabilities, Status, Heartbeat}`
- Registration, heartbeat (30s), deregistration, stale detection
- Capability-based selection + preferred-agent routing
- Filesystem-backed store (`storage/filesystem/agent_registry.go`)

### Controller Serve Command
- `stratum-controller serve --listen :8080 --data-dir ./data`
- `POST /v1/agents/register` — agent registration
- `POST /v1/agents/heartbeat` — heartbeat
- `GET /v1/agents` — list all agents
- `GET /healthz` — health check

---

## ✅ Phase 6c — Container Orchestration (Complete)

### Docker Compose
- `Dockerfile.agent` — parameterized `ARG JAVA_VERSION` (8/17/21), eclipse-temurin
- `Dockerfile.controller` — standalone controller image (not in compose)
- `docker-compose.yml` — 3 agents (java8:8787, java17:8788, java21:8789)
- Agents connect to host controller via `host.docker.internal:8080`
- `.env.example` + `.dockerignore`

### Architecture
```
Host                                     Docker
┌──────────────────────┐    ┌───────────────────────────┐
│ stratum-controller   │    │ agent-java8    (0.0.0.0:8787)│
│  serve --listen :8080│◄───│ agent-java17   (0.0.0.0:8788)│
│  --data-dir ./data   │    │ agent-java21   (0.0.0.0:8789)│
│                      │    │                           │
│ Registry: agents/    │    │ Each auto-registers       │
│ Agent 8 → 1.12.2    │    │ Heartbeat every 30s        │
│ Agent 17 → 1.17.1   │    │ Isolated Java runtime      │
│ Agent 21 → latest   │    │ Isolated /runtime volume   │
└──────────────────────┘    └───────────────────────────┘
```

---

## ⚠️ Not Yet Implemented

### Runtime Execution
- Real Minecraft boot via MCDR (tests use helper process stubs)
- ReadinessCheck with real Minecraft server logs

### World Management
- Cross-agent restore (agent-ownership validation blocks it)
- Incremental/differential backup

### Environments
- Modern Forge (1.13+) / NeoForge
- LiteLoader 1.12

### Infrastructure
- **Web UI** — Skipped (Phase 5)
- **Authentication & Authorization** — Shared-token only, no user accounts, no RBAC

---

## 🔐 Safety Status

- RuntimeProfile uses declarative JSON only; no shell commands
- Agent rejects path-traversal in session/artifact names
- Artifact SHA-256 verification before materialization + apply
- Artifact approval workflow enforced
- Session runtime directories isolated under `--runtime-root`
- World zip-restore: zip-slip protection (symlinks, `..`, absolute paths)
- Pre-operation checkpoints before restart and artifact apply
- Checkpoint restore requires session in stopped state
- Agent containers isolated with named volumes

---

## 📊 Verification

```
gofmt -l internal/ cmd/       # ✅
go build ./...                 # ✅
go test -count=1 ./...        # ✅ (54+ packages)
go vet ./...                   # ✅
```

---

## 📋 Suggested Next Task

**Phase 6b: Authentication & Authorization** — User accounts, RBAC, project membership, replace shared-token auth.
