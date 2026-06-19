# Implementation Status

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
- Standard-library CLI (no external frameworks)
- Resource-aware session scheduling

### Agent
- HTTP transport for Controller-Agent communication
- Per-session runtime directory allocation
- RuntimeProfile registry and validation
- Process supervision with dummy RuntimeProfile
- Terminal executor with bounded logs and exit tracking

## ✅ Lucy Integration

### Status: **Production-ready as Go library**

- Embedded as `github.com/mclucy/lucy` direct dependency
- `internal/integration/lucy` adapter layer isolates domain model
- `EmbeddedAdapter` calls Lucy library functions directly
- All tests passing

### Capabilities

| Feature | Status | Implementation |
|---------|--------|----------------|
| Manifest parsing | ✅ Complete | `ManifestService` |
| Dependency resolution | ✅ Complete | `PlanEnvironment` |
| Lock generation | ✅ Complete | `LockEnvironment` |
| Package installation | ✅ Complete | `InstallPackages` |
| Integrity verification | ✅ Complete | `VerifyIntegrity` |
| Status checking | ✅ Complete | `CheckStatus` |
| Artifact analysis | ✅ Complete | `artifact.go` |

### Integration Points

- Environment materialization calls Lucy for package planning
- Session start validates Lucy lock files
- Checkpoint metadata includes Lucy lock hash
- CLI exposes `environments validate-manifest` command

### Limitations

- Lucy does not manage JVM processes
- Lucy does not start Minecraft or MCDR
- Lucy does not perform runtime lifecycle operations
- Manifest/lock file writing not yet integrated into Environment service

## ✅ MCDR Integration

### Status: **Planning contract complete, executor pending**

- `internal/agent/mcdrbridge` defines launch plan contract
- RuntimeProfile `mcdr-fabric-1.17.json` exists in `runtime-profiles/`
- MCDR layout helpers implemented in `internal/agent/process`
- All bridge tests passing

### Capabilities

| Feature | Status | Implementation |
|---------|--------|----------------|
| Launch plan construction | ✅ Complete | `BuildLaunchPlan` |
| Launch plan validation | ✅ Complete | `ValidateLaunchPlan` |
| Launch plan inspection | ✅ Complete | `InspectLaunchPlan` |
| MCDR layout derivation | ✅ Complete | `MCDRRuntimeLayout` |
| Directory preparation | ✅ Complete | `process.MaterializeEnvironment` |
| RuntimeProfile executor | ⚠️ Pending | Future task |

### Launch Plan Manifest

- Persisted at `work/mcdr/mcdr-launch-plan.json`
- Records environment identity, canonical paths, launch command, stop strategy
- Validated during Session start readiness check
- Agent reads plan but does not yet execute it

### Ownership Boundaries

- Agent owns MCDR process supervision (start, stop, logs, exit)
- MCDR manages Minecraft internally via config.yml start_command
- Controller owns Session metadata and lifecycle decisions
- MCDR bridge is planning-only; does not start processes

## ⚠️ Not Yet Implemented

### Runtime Execution
- **MCDR RuntimeProfile executor** — Agent does not yet launch MCDR processes
- **Real JVM process** — No actual Minecraft server started
- **Java runtime detection** — Version validation exists but no real JVM discovery
- **Server jar provisioning** — Fabric/Forge download not implemented

### Lucy Environment Integration
- **Manifest generation** — Environment does not yet write lucy.yaml
- **Lock file generation** — Environment does not yet write lucy-lock.yaml
- **Package installation** — Materialization does not yet call Lucy InstallPackages
- **Dependency updates** — No workflow for updating locked packages

### World Management
- **Checkpoint backup** — No world file copy/archive
- **Checkpoint restore** — No world rollback
- **Chunk regeneration** — Not planned for MVP

### Additional Environments
- **1.12 support** — Only 1.17 planned for MVP
- **Latest version** — Not yet defined
- **Forge/NeoForge** — Only Fabric implemented

### Infrastructure
- **Web UI** — CLI-only currently
- **Container orchestration** — No deployment tooling
- **Multi-Agent coordination** — Single local Agent only
- **Authentication** — Shared-token only, no user accounts

## 🔐 Safety Status

### Implemented Safety Boundaries

- RuntimeProfile uses declarative JSON configs only
- No CLI accepts executable paths or shell commands
- Agent rejects unsafe session/artifact names (path traversal)
- Artifact payloads verified with SHA-256 before materialization
- Artifact approval workflow enforced before apply
- Session runtime directories isolated under `--runtime-root`
- All tests use process stubs; no real JVM/shell execution

### Test Coverage

```
go test ./...
```

All core packages have passing tests:
- Domain model validation
- Repository CRUD operations
- Operation idempotency and correlation
- RuntimeProfile validation
- Artifact staging and apply workflows
- Lucy adapter integration
- MCDR bridge planning

## 📋 Next Atomic Tasks

1. **MCDR RuntimeProfile executor v0**
   - Agent `process.Supervisor` executes MCDR command_argv
   - Reads MCDR bridge launch plan
   - Supervises MCDR process (start, stop, logs, exit)
   - MCDR launches Minecraft via config.yml
   - Tests use stub MCDR config (no real Minecraft)

2. **Environment → Lucy manifest generation**
   - Environment service writes lucy.yaml from Environment metadata
   - Session start calls Lucy PlanEnvironment
   - Lock file persisted in session runtime directory
   - Checkpoint includes Lucy lock hash

3. **Java runtime detection**
   - Detect installed Java versions on Agent machine
   - Validate Environment.JavaVersion against detected runtimes
   - RuntimeProfile includes Java path in launch command

4. **Server jar provisioning**
   - Download Fabric/Forge server jar
   - Verify checksum
   - Place in session runtime directory

5. **World checkpoint backup**
   - Copy world/ directory to checkpoint storage
   - Record world snapshot metadata
   - Checkpoint restore copies world/ back

## 📊 Verification Status

**All verification passed:**

```powershell
gofmt -l .                     # ✅ No formatting issues
go test -count=1 ./...         # ✅ All tests passing
git diff --check               # ✅ No whitespace errors
```

**Lucy integration verified:**

```powershell
.\lucy.exe --version           # ✅ Commit 3247a5840b
go test ./internal/integration/lucy/...  # ✅ ok 0.563s
```

**MCDR bridge verified:**

```powershell
go test ./internal/agent/mcdrbridge/...  # ✅ ok 0.394s
go test ./internal/agent/process/...     # ✅ ok 1.151s
```

**RuntimeProfile registry verified:**

```powershell
go test ./internal/agent/runtimeprofile/...  # ✅ ok 0.290s
```

Last updated: 2026-06-19
