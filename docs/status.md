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
- HTTP API for all domain entities
- Standard-library CLI (cobra-free, flag-based)
- Resource-aware session scheduling

### Agent
- HTTP transport for Controller-Agent communication
- Per-session runtime directory allocation
- RuntimeProfile registry and validation
- Real OS process supervision (dummy, terminal, MCDR)
- Bounded log capture with circular buffer
- Process crash detection and graceful stop

---

## ✅ Lucy Integration

### Status: Production-ready as Go library

- Embedded as `github.com/mclucy/lucy` direct dependency
- `internal/integration/lucy` adapter layer isolates domain model
- `EmbeddedAdapter` calls Lucy library functions directly (default)
- `NoopAdapter` available when Lucy is disabled (`STRATUM_LUCY_WORKSPACE=none`)

### Capabilities

| Feature | Status | Implementation |
|---------|--------|----------------|
| Manifest parsing | ✅ Complete | `ManifestService` |
| Manifest generation | ✅ Complete | `MaterializeEnvironment` writes `lucy.yaml` |
| Dependency resolution | ✅ Complete | `PlanEnvironment` |
| Lock generation | ✅ Complete | `LockEnvironment` → `lucy-lock.yaml` |
| Package installation | ✅ Complete | `InstallPackages` during materialization |
| Integrity verification | ✅ Complete | `VerifyIntegrity` at start readiness |
| Status checking | ✅ Complete | `CheckStatus` |
| Artifact analysis | ✅ Complete | `artifact.go` |

### Integration Points

- Environment materialization calls Lucy for package planning, lock, install, and integrity
- Session start validates Lucy lock files and integrity
- Checkpoint metadata includes Lucy lock hash
- CLI exposes `lucy validate-manifest`, `lucy plan`, `lucy lock`, `lucy install` commands

---

## ✅ MCDR Integration

### Status: Fully implemented

- `internal/agent/mcdr/supervisor.go` — MCDR lifecycle (Start/Stop/Restart/SendCommand)
- `internal/agent/process/process.go` — `startTerminal` launches real OS processes, `waitTerminal` monitors exit, `StopProcess` performs graceful stop
- `internal/agent/local/process_agent.go` — routes `StartSession` to `a.mcdr.Start()` for MCDR profiles
- `runtime-profiles/mcdr-fabric-1.17.json` — declarative profile with stdin stop strategy and log-pattern readiness check
- `internal/agent/mcdrbridge` — launch plan contract and validation

### Capabilities

| Feature | Status | Implementation |
|---------|--------|----------------|
| MCDR process start/stop | ✅ Complete | `mcdr.Supervisor.Start/Stop` |
| stdin command injection | ✅ Complete | `mcdr.Supervisor.SendCommand` |
| Log-pattern readiness check | ✅ Complete | `process.Supervisor.WaitForLog` |
| Graceful stop (stdin→force kill) | ✅ Complete | `process.Supervisor.StopProcess` |
| Crash detection | ✅ Complete | `waitTerminal` goroutine |
| MCDR config.yml generation | ✅ Complete | `config_writer.go` |
| Launch plan construction | ✅ Complete | `mcdrbridge.BuildLaunchPlan` |
| Python venv + MCDR install | ✅ Complete | `process.materializeMCDRRuntime` |

### Ownership Boundaries

- Agent owns MCDR process supervision (start, stop, logs, exit)
- MCDR manages Minecraft internally via config.yml `start_command`
- Controller owns Session metadata and lifecycle decisions

---

## ✅ Java Runtime Detection

### Status: Fully implemented

- `internal/agent/java/detector.go` — `SelectForMinecraftVersion` detects installed JVMs
- `internal/agent/process/process.go` — `materializeJavaAndServerJar` wires detection into materialization
- Detected metadata: executable path, version, major version, JAVA_HOME
- Fallback handling when detection fails

---

## ✅ Server Jar Provisioning

### Status: Fully implemented

- `internal/agent/serverjar/downloader.go` — Vanilla, Fabric, and Paper downloads
- SHA-256 checksum verification on deploy
- Proxy support via `STRATUM_PROXY` / `--http-proxy`
- Local cache directory (`<runtime-root>/cache/serverjars`)
- Deploys to session work directory during materialization

---

## ✅ Phase 2 — Runtime Execution (Complete)

All six priorities of Phase 2 are implemented:

1. **MCDR RuntimeProfile Executor v0** — Real OS process execution via `startTerminal`
2. **Proxy Configuration** — `SetProxy()` + `--http-proxy` CLI flag
3. **Lucy Manifest Generation** — `lucy.yaml` written during materialization
4. **Lucy Package Installation** — `InstallPackages` + `VerifyIntegrity`
5. **Java Runtime Detection** — JVM discovery and version validation
6. **Server Jar Provisioning** — Vanilla/Fabric/Paper downloads with SHA-256 checksums

---

## ✅ Phase 3 — World Management (Complete)

### World Checkpoint Backup/Restore

- `internal/agent/worldcheckpoint/checkpoint.go` — `Worker.Create` (zip + SHA-256), `Worker.Restore` (unzip with slip protection)
- `internal/agent/worldcheckpoint/ref.go` — Agent-local snapshot reference scheme
- `internal/checkpoint/service/service.go` — Full orchestration for all four consistency levels
- `internal/cli/handlers_checkpoints.go` — create/list/inspect/restore/diff commands

### Four Consistency Levels

| Level | Behavior | Status |
|-------|----------|--------|
| `metadata_only` | Metadata and runtime status only | ✅ |
| `stopped` | Stop session → snapshot → restart | ✅ |
| `best_effort` | `save-all flush` + world snapshot | ✅ |
| `command_quiesced` | `save-off` → `save-all flush` → snapshot → `save-on` | ✅ |

### Restore Orchestration

- `--auto-stop` / `--auto-start` CLI flags for full lifecycle management
- World Profile application during restore (full or partial field merge)
- Checkpoint diff command for world profile comparison
- Zip-slip protection on restore

### Pre-Operation Checkpoints

- `sessions restart --pre-op-checkpoint` — creates world snapshot before restart
- `artifacts apply execute --pre-op-checkpoint` — creates world snapshot before file copy
- Shared `createPreOpCheckpoint` helper in `internal/cli/handlers_shared.go`
- KindPreOperation checkpoints with WorldStateRef and audit trail
- Best-effort: checkpoint failure logs warning but does not block the operation

---

## ⚠️ Not Yet Implemented

### Runtime Execution
- **Real Minecraft server** — No actual Minecraft server started by MCDR (tests use helper process stubs)
- **ReadinessCheck with real MCDR** — Log-pattern readiness check defined but not tested with real Minecraft boot

### World Management
- **Cross-agent restore** — Currently rejected (agent-ownership validation)
- **Incremental backup** — Full-world zip only; no differential backup
- **Chunk regeneration** — Not planned for MVP

### Additional Environments
- **1.12 support** — Only 1.17 implemented
- **Latest version** — Not yet defined
- **Forge/NeoForge** — Only Fabric implemented

### Infrastructure
- **Web UI** — CLI-only currently
- **Container orchestration** — No deployment tooling
- **Multi-Agent coordination** — Single local Agent only
- **Authentication** — Shared-token only, no user accounts

---

## 🔐 Safety Status

### Implemented Safety Boundaries

- RuntimeProfile uses declarative JSON configs only
- No CLI accepts executable paths or shell commands
- Agent rejects unsafe session/artifact names (path traversal)
- Artifact payloads verified with SHA-256 before materialization
- Artifact approval workflow enforced before apply
- Session runtime directories isolated under `--runtime-root`
- World zip-restore has zip-slip protection (symlinks, `..`, absolute paths rejected)
- Pre-operation checkpoints before dangerous operations (restart, artifact apply)
- Checkpoint restore requires session in stopped state (JVM file lock safety)
- Best-effort and command_quiesced levels flush chunks before snapshot

### Test Coverage

All core packages have passing tests:
- Domain model validation
- Repository CRUD operations
- Operation idempotency and correlation
- RuntimeProfile validation and registry
- Artifact staging and apply workflows
- Lucy adapter integration (embedded, noop, CLI modes)
- MCDR bridge planning and supervision
- World checkpoint create/restore with zip-slip protection
- Session lifecycle with pre-op checkpoints
- CLI command handlers with HTTP transport

---

## 📊 Verification Status

```
gofmt -l .                     # ✅ No formatting issues
go test -count=1 ./...         # ✅ All tests passing (54 packages)
go vet ./...                   # ✅ No issues
```

---

## 📋 Suggested Next Task

**Phase 4: 1.12 Support** — implement Forge 1.12.2 loader support, legacy Carpet mod compatibility, Lucy manifest compatibility for 1.12 ecosystem.
