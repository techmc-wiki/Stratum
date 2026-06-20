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

## 🚧 Phase 2: Runtime Execution (In Progress)

### Priority 1: MCDR RuntimeProfile Executor v0

**Goal:** Agent can launch and supervise MCDR processes (no real Minecraft yet).

**Tasks:**
1. Implement `process.Supervisor.startMCDR` method
   - Read MCDR bridge launch plan from `work/mcdr/mcdr-launch-plan.json`
   - Execute `mcdreforged --start` (from RuntimeProfile command_argv)
   - Apply stdin stop strategy: `!!MCDR stop`
   - Capture stdout/stderr logs
   - Track process PID and exit code

2. Create test MCDR configuration
   - Stub config.yml with `start_command: echo "stub Minecraft"`
   - Verify Agent can start/stop/observe MCDR process lifecycle

3. Integrate into Session lifecycle
   - Session start selects `mcdr-fabric-1.17` RuntimeProfile
   - Environment materialization calls MCDR bridge BuildLaunchPlan
   - Agent executes MCDR process and reports status

**Verification:**
```bash
stratum sessions create --id test --project p1 --room r1
stratum sessions start --id test --runtime-profile mcdr-fabric-1.17
stratum sessions inspect --id test  # runtimeMessage shows MCDR process
stratum sessions logs --id test     # shows MCDR logs
stratum sessions stop --id test     # graceful MCDR stop
```

**Atomic commits:**
- `runtime: implement MCDR RuntimeProfile executor v0`
- `test: add MCDR stub config for supervision tests`
- `lifecycle: integrate MCDR executor into session start/stop`

**Estimated effort:** 2-3 days

---

### Priority 2: Proxy Configuration for ServerJar Downloads

**Goal:** Support HTTP proxy for Mojang/Fabric/Paper API access in restricted networks.

**Tasks:**
1. Add proxy configuration to Agent config
   - `--http-proxy` CLI flag (e.g., `http://127.0.0.1:10808`)
   - Environment variable `STRATUM_HTTP_PROXY`
   - Call `serverjar.SetProxy()` during Agent init

2. Update serverjar downloader documentation
   - Document proxy configuration in README
   - Add network requirements section

**Verification:**
```bash
stratum-agent serve --http-proxy http://127.0.0.1:10808
# Verify ServerJar downloads succeed through proxy
```

**Atomic commit:**
- `agent: add HTTP proxy configuration for serverjar downloads`

**Estimated effort:** 0.5 day

---

### Priority 3: Environment → Lucy Manifest Generation

**Goal:** Environment writes lucy.yaml and generates lucy-lock.yaml.

**Tasks:**
1. Add `environment.WriteManifest()` method
   - Convert Environment metadata to Lucy manifest format
   - Write to `<session-runtime-dir>/lucy.yaml`

2. Session start integration
   - After materialization, call Lucy `PlanEnvironment`
   - Generate and persist lucy-lock.yaml
   - Record lock hash in Environment materialization manifest

3. Checkpoint integration
   - Read lucy-lock.yaml during checkpoint creation
   - Store lock hash in Checkpoint metadata
   - Verify lock hash consistency on restore

**Verification:**
```bash
stratum environments create --id env-1 --minecraft-version 1.17.1 --loader fabric
stratum sessions start --id test --environment env-1
# Check <runtime-root>/test/lucy.yaml exists
# Check <runtime-root>/test/lucy-lock.yaml exists
stratum checkpoints create --session test --id cp1
stratum checkpoints inspect --id cp1 | grep lockHash
```

**Atomic commits:**
- `environment: add Lucy manifest generation from Environment metadata`
- `lifecycle: generate Lucy lock during session start`
- `checkpoint: record and verify Lucy lock hash`

**Estimated effort:** 2 days

---

### Priority 4: Lucy Package Installation

**Goal:** Environment materialization actually downloads and installs mods/plugins.

**Tasks:**
1. Call Lucy `InstallPackages` during materialization
   - Read lucy-lock.yaml packages
   - Install to `<session-runtime-dir>/mods/`
   - Record installation result in materialization manifest

2. Integrity verification
   - Session start readiness calls Lucy `VerifyIntegrity`
   - Report missing or corrupted packages
   - Block session start when integrity fails

**Verification:**
```bash
# With lucy.yaml containing fabric-api and carpet
stratum sessions start --id test
# Check <runtime-root>/test/mods/ contains downloaded jars
stratum sessions inspect --id test | grep integrityStatus
```

**Atomic commits:**
- `lucy: call InstallPackages during environment materialization`
- `readiness: add Lucy integrity verification to start gate`

**Estimated effort:** 1-2 days

---

### Priority 5: Java Runtime Detection

**Goal:** Discover installed Java versions and validate compatibility.

**Tasks:**
1. Implement `internal/agent/java` real detection
   - Check JAVA_HOME
   - Scan common installation paths (Windows: Program Files, Linux: /usr/lib/jvm)
   - Parse `java -version` output

2. Environment compatibility validation
   - Session start verifies Java version matches Environment requirement
   - Report incompatible Java versions with actionable error

**Verification:**
```bash
stratum-agent serve
# Agent startup logs discovered Java versions
stratum sessions start --id test --environment env-1
# Start fails with clear error if Java version mismatch
```

**Atomic commits:**
- `java: implement JVM discovery and version parsing`
- `readiness: add Java version compatibility check`

**Estimated effort:** 1-2 days

---

### Priority 6: Server Jar Provisioning

**Goal:** Download and verify Minecraft server jars.

**Tasks:**
1. Complete serverjar download implementations
   - Vanilla: Mojang version manifest (partially done)
   - Fabric: Fabric maven repository
   - Paper: Paper API (partially done)

2. Deploy to session runtime directory
   - Write to `<session-runtime-dir>/server.jar`
   - Verify SHA-256 checksum
   - Record jar metadata in materialization manifest

**Verification:**
```bash
stratum sessions start --id test
# Check <runtime-root>/test/server.jar exists
# Verify checksum matches official source
```

**Atomic commits:**
- `serverjar: complete Vanilla and Fabric download implementation`
- `lifecycle: deploy server jar during environment materialization`

**Estimated effort:** 2 days

---

## 🔮 Phase 3: World Management

### Priority 7: World Checkpoint Backup/Restore

**Goal:** Actually copy and restore world files.

**Tasks:**
1. Implement world backup
   - Copy `<session-runtime-dir>/world/` to `<checkpoints-dir>/<checkpoint-id>/world/`
   - Record world size and file count

2. Implement world restore
   - Stop Session
   - Replace world/ directory from checkpoint
   - Restart Session

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
