# StratumMC Development Roadmap

## Current Status (2026-06-19)

### ✅ Phase 1: Core Infrastructure (Complete)

- Domain models and explicit state transitions
- Filesystem-backed storage with audit history
- Durable operation coordination with idempotency
- HTTP Controller-Agent transport
- RuntimeProfile registry and validation
- Process supervision framework (dummy RuntimeProfile)
- Artifact staging and apply workflows
- Environment materialization pipeline
- **Lucy integration** — embedded Go library for dependency resolution, lock generation, package installation
- **MCDR bridge** — planning contract for MCDR child RuntimeProfile

**Test Coverage:** Core domain, storage, session lifecycle, Lucy adapter, MCDR bridge all passing.

**Safety:** All tests use process stubs. No real JVM/shell/MCDR execution.

---

## ✅ Phase 2: Runtime Execution (Complete)

### ✅ Priority 1: MCDR RuntimeProfile Executor v0

**Status: Complete.** Implementation: `mcdr.Supervisor.Start` -> `process.Supervisor.StartProcess` -> `startTerminal` (real OS `exec.Command`).

**Key files:**
- `internal/agent/mcdr/supervisor.go` — MCDR lifecycle (Start/Stop/Restart/SendCommand)
- `internal/agent/process/process.go` — `startTerminal` (OS process launch), `waitTerminal` (exit monitoring), `StopProcess` (graceful stop via stdin/signal/kill)
- `internal/agent/local/process_agent.go` — routes `StartSession` to `a.mcdr.Start()` for MCDR profiles
- `runtime-profiles/mcdr-fabric-1.17.json` — stop_strategy: stdin, readiness: log-pattern "Done ("

**Tests:**
```
TestMCDRSupervisorStartStop          — real OS process lifecycle
TestMCDRSupervisorRestart            — new PID after restart
TestMCDRSupervisorSendCommand        — stdin command injection
TestStartCommandUsesJavaExecutable   — config.yml generation
```

### ✅ Priority 2: Proxy Configuration for ServerJar Downloads

**Status: Complete.** `serverjar.SetProxy()` in `process.go:203`, `--http-proxy` CLI flag in `cmd/stratum-agent/main.go`.

### ✅ Priority 3: Environment → Lucy Manifest Generation

**Status: Complete.** `process.go:935-1001` MaterializeEnvironment writes `lucy.yaml` and `lucy-lock.yaml`.

### ✅ Priority 4: Lucy Package Installation

**Status: Complete.** `process.go:1003-1026` calls Lucy `InstallPackages`, `process.go:1028-1061` calls `VerifyIntegrity`.

### ✅ Priority 5: Java Runtime Detection

**Status: Complete.** `internal/agent/java/detector.go` — detects installed JVM, validates compatibility.

### ✅ Priority 6: Server Jar Provisioning

**Status: Complete.** `internal/agent/serverjar/downloader.go` — Vanilla/Fabric/Paper downloads.

---

## 🚧 Phase 3: World Management (In Progress)

### ✅ Priority 7: World Checkpoint Backup/Restore

**Status: Core complete. CLI orchestration added.**

**Implemented:**
- World snapshot creation (zip + SHA-256) — `internal/agent/worldcheckpoint/checkpoint.go`
- World snapshot restore (unzip with zip-slip protection) — `internal/agent/worldcheckpoint/checkpoint.go`
- Three consistency levels: `metadata_only`, `stopped`, `best_effort`, `command_quiesced`
- `stopped` level: Stop session → snapshot → restart — `internal/checkpoint/service/service.go`
- CLI restore orchestration: `--auto-stop` and `--auto-start` flags — `internal/cli/handlers_checkpoints.go`
- World profile application during restore (full or partial merge)
- Agent-local snapshot reference scheme (`agent-local://agent/sessions/...`)
- Checkpoint diff for comparing world profiles

**Key files:**
- `internal/agent/worldcheckpoint/checkpoint.go` — `Worker.Create` (zip+hash), `Worker.Restore` (unzip)
- `internal/agent/worldcheckpoint/ref.go` — Snapshot reference building/parsing
- `internal/checkpoint/service/service.go` — `createStopped`, `createBestEffort`, `createCommandQuiesced`, `Restore`
- `internal/cli/handlers_checkpoints.go` — create/list/inspect/restore/diff commands

**Remaining:**
- Cross-agent snapshot restore (currently rejected)
- Incremental/differential backup (always full-world zip)
- Pre-operation automatic checkpoint creation
- Chunk regeneration (explicitly out of scope)

**Verification:**
```bash
stratum sessions start --id test
# Make changes in Minecraft
stratum checkpoints create --session test --id cp1
# Make more changes
stratum checkpoints restore --id cp1 --session test
# Verify world reverted to cp1 state
```

**Atomic commits:**
- `worldcheckpoint: implement world backup during creation`
- `checkpoint: implement world restore from checkpoint`

**Estimated effort:** 2-3 days

---

## 🌐 Phase 4: Additional Environments

### 1.12 Support

- Forge 1.12.2 loader support
- Legacy Carpet mod
- Lucy manifest compatibility

**Estimated effort:** 1 week

### Latest Version Support

- Track Mojang version manifest for latest release
- Auto-update Environment templates
- Test with latest Fabric/Paper

**Estimated effort:** 1 week

---

## 🖥️ Phase 5: User Interface

### Web UI

- REST API endpoints (already exist)
- Frontend framework (React/Svelte/HTMX)
- Real-time session status updates (WebSocket)
- Artifact upload interface
- Checkpoint browser

**Estimated effort:** 3-4 weeks

---

## 🔐 Phase 6: Production Readiness

### Multi-Agent Coordination

- Agent registry and discovery
- Session placement across multiple nodes
- Resource balancing

**Estimated effort:** 2-3 weeks

### Authentication & Authorization

- User account system
- Role-based access control (RBAC)
- Project membership management
- Replace shared-token authentication

**Estimated effort:** 2 weeks

### Container Orchestration

- Docker/Podman support
- Kubernetes deployment manifests
- Persistent volume management

**Estimated effort:** 1-2 weeks

---

## Timeline Estimate

| Phase | Duration | Completion Target |
|-------|----------|-------------------|
| Phase 2: Runtime Execution | 2-3 weeks | 2026-07-10 |
| Phase 3: World Management | 1 week | 2026-07-17 |
| Phase 4: Additional Environments | 2 weeks | 2026-07-31 |
| Phase 5: Web UI | 4 weeks | 2026-08-28 |
| Phase 6: Production Readiness | 5 weeks | 2026-10-02 |

**First usable testing platform (Phase 2 complete):** ~2026-07-10

**Full MVP with world management:** ~2026-07-31

**Production-ready with Web UI:** ~2026-10-02

---

## Non-Goals (Out of Scope)

- Automatic URL mixin source compilation
- Automatic world merge from forked sessions
- Generic game panel features
- Public registration/hosting marketplace
- Broad modpack hosting
- Alternative JVM languages (Kotlin, Scala mods)

---

## Dependencies & Blockers

### External Dependencies
- Lucy library updates (currently stable)
- MCDR Python package availability
- Mojang/Fabric/Paper API stability

### Network Requirements
- HTTP/HTTPS access to:
  - launchermeta.mojang.com
  - maven.fabricmc.net
  - api.papermc.io
  - modrinth.com (for Lucy package resolution)
- Proxy support: `--http-proxy http://127.0.0.1:10808`

### System Requirements
- Java 8+ for 1.12
- Java 16+ for 1.17
- Java 21+ for latest
- Python 3.9+ for MCDR
- 2GB+ RAM per session (configurable)

---

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| MCDR process instability | Medium | High | Agent force-kill timeout, crash detection |
| Network download failures | High | Medium | Proxy support, retry logic, local cache |
| Java version conflicts | Medium | Medium | Explicit version detection, clear error messages |
| World corruption during checkpoint | Low | High | Atomic file operations, pre-checkpoint validation |
| Lucy dependency resolution conflicts | Medium | Medium | Lock file pinning, manual override support |

---

## Success Metrics

### Phase 2 Complete
- [ ] Agent can start/stop MCDR processes
- [ ] Session logs show MCDR output
- [ ] Proxy configuration works for restricted networks

### Phase 3 Complete
- [ ] World checkpoint/restore works end-to-end
- [ ] No data loss during restore operations

### MVP Complete (Phase 4)
- [ ] 1.12 and 1.17 environments both functional
- [ ] Lucy package installation works for common mods
- [ ] 5+ concurrent sessions without crashes

### Production Ready (Phase 6)
- [ ] Web UI accessible and functional
- [ ] Multi-user authentication works
- [ ] 20+ concurrent sessions across multiple agents
- [ ] <1% session crash rate

---

## Next Immediate Task

**Start:** MCDR RuntimeProfile Executor v0

**Owner:** Development team

**Target:** 2026-06-22 (3 days)

**Deliverables:**
- `internal/agent/process/process.go` with `startMCDR` method
- Test MCDR config with stub Minecraft launch
- Integration tests passing
- CLI commands functional

**Success Criteria:**
```bash
stratum sessions start --id test --runtime-profile mcdr-fabric-1.17
# Returns success
stratum sessions inspect --id test
# Shows runtimeMessage: "running"
stratum sessions logs --id test
# Shows MCDR startup logs
stratum sessions stop --id test
# Graceful stop via stdin "!!MCDR stop"
```
