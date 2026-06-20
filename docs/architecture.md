# Architecture

StratumMC is a CLI-first Minecraft technical-testing control plane organized around long-lived Projects, shared Rooms, semantic Checkpoints, and isolated Sessions. It is not a generic server panel and does not treat a Minecraft server process as the top-level product object. The primary domain flow is:

```text
Project -> Room -> Session -> Checkpoint / Fork / Artifact / Environment
```

A Project represents a long-term collaboration unit such as a lab or engineering group. A Room represents a shared testing workspace inside a Project. A Session is an actual runnable server instance. Checkpoints capture semantic experiment snapshots that can later be restored, compared, or forked into isolated test sessions.

Domain packages contain policy and metadata only. Process execution, container management, filesystem mutation, MCDR daemon control, Minecraft-side bridge operations, and checkpoint packing live behind Agent-side integration interfaces.

See [CLI Reference](cli-reference.md) for complete command documentation.

## Core principles

StratumMC follows these architectural rules:

1. The Controller owns authoritative metadata, Operations, permissions, scheduling decisions, and audit history.
2. Agents own machine-local execution, runtime directories, processes, containers, logs, and bridge manifests.
3. RuntimeProfile describes how a Session is executed; Environment describes what Minecraft environment the Session requires.
4. MCDR is treated as a daemon wrapper and plugin transport, not as the top-level Stratum lifecycle controller.
5. Strong world-state checkpointing requires a Minecraft-side bridge, not only MCDR.
6. Lucy is a dependency/package planning integration, not a runtime owner.
7. Artifacts are governed by Stratum approval, staging, materialization, apply, and verification rules.
8. Dangerous operations must be explicit Operations and must produce audit events.

## Product domain model

### Project

A Project is the long-term collaboration boundary. Examples include `117lab`, `112lab`, or other technical-testing groups.

A Project owns:

* Rooms
* members and roles
* resource policies
* permission policies
* project-scoped Artifacts
* project-scoped Environments
* audit history
* operation history

Project metadata should not imply any specific runtime host. A Project may contain many Rooms and many Sessions spread across multiple Agents.

### Room

A Room is a shared workspace inside a Project. Examples include `1.12-main-flat`, `1.12-main-void`, or `1.17-carpet-test`.

A Room may define:

* a default Environment
* a default RuntimeProfile requirement
* a default World Profile
* base Artifacts
* shared configuration
* default Carpet rules
* checkpoint policy
* fork policy
* resource policy
* a default shared Session

A Room normally has one primary shared Session used by collaborators as the main testing server. Users may fork from the Room or from one of its Checkpoints to perform dangerous experiments without damaging the shared world.

Room creation must validate referenced Environment metadata before persistence. Repositories remain storage-only; validation belongs at service, CLI, or API boundaries.

### Session

A Session is an actual runnable server instance. It is the runtime object that can be prepared, started, stopped, restarted, checkpointed, forked, archived, or deleted.

Session types include:

* `shared`: the long-lived shared instance for a Room
* `fork`: a temporary experiment instance created from a Room, Session, or Checkpoint
* `private`: a short-lived personal sandbox
* `review`: an isolated instance used to test uploaded Artifacts such as jars or plugins
* `archived`: a preserved, non-running Session record

A Session records:

* Project ID
* Room ID when applicable
* Session type
* source Session or source Checkpoint when forked
* Environment ID
* RuntimeProfile ID
* World Profile or world reference
* applied Artifacts
* Agent assignment
* runtime status metadata
* current lifecycle state
* operation history

Authoritative Session state is stored by the Controller. Agent observations do not directly mutate this state. Controller services decide whether and when to persist a state transition.

### Fork Session

A Fork Session is an isolated Session created from a Room, existing Session, or Checkpoint. Its purpose is to support dangerous experiments without corrupting the shared Room state.

A fork should preserve source provenance:

* source Project
* source Room
* source Session
* source Checkpoint
* creator
* reason
* creation Operation
* inherited Environment
* inherited Artifact set
* inherited server config
* inherited world state reference

Fork creation should eventually be coordinated with checkpoint creation. A dangerous fork from a running shared Session should either require a recent Checkpoint or create a pre-fork Checkpoint according to policy.

### Checkpoint

A Checkpoint is a semantic experiment snapshot. It is not merely a filesystem backup. It records enough metadata to explain what was captured, why it was captured, and how it can be restored or forked.

A Checkpoint may record:

* world state reference
* Environment
* RuntimeProfile
* Lucy lock hash or dependency lock reference
* server config
* Carpet rules
* applied Artifacts
* seed
* generator settings
* World Profile
* source Project
* source Room
* source Session
* creator
* note
* operation history
* consistency level
* runtime-status snapshot
* backup manifest or storage reference

Checkpoint consistency levels are explicit:

* `metadata_only`: metadata and runtime-status only
* `stopped`: runtime stopped before snapshot
* `best_effort`: commands such as `save-all` used before copying
* `command_quiesced`: Agent sends `save-off`, `save-all flush` to MCDR stdin (or Minecraft console), parses stdout for confirmation, snapshots world, sends `save-on`. **MCDR acts as command transport**. Does not guarantee full internal consistency (mod state, async tasks).
* `plugin_backup`: Agent sends plugin command (e.g., `!!backup create`) to MCDR stdin, waits for plugin to write backup metadata, verifies backup integrity. **MCDR plugin performs backup**. Consistency depends on plugin quality.
* `mc_bridge_prepared`: Agent calls MC Bridge `Prepare()` to freeze world state, snapshots, calls `Commit()`/`Abort()`. **Requires MC Bridge/Debug Mod**. Strongest consistency. MCDR may transport bridge messages but does not implement consistency itself.

Current checkpoint support is metadata-oriented. Future world snapshots, restore, fork, and strong consistency require Agent-side checkpoint orchestration and possibly a Minecraft-side bridge.

### Artifact

An Artifact is a managed upload or external resource used by a Session. Examples include mod jars, MCDR plugins, config files, datapacks, resource packs, schematics, world templates, server jars, scripts, or future Debug Mod builds.

The Artifact pipeline is controlled by Stratum:

```text
metadata create -> import/blob -> approve/reject -> staging plan -> materialize -> apply plan -> apply execute -> verify
```

Unapproved Artifacts must not enter shared Sessions. Review Sessions exist to test uploaded Artifacts in isolation before approval.

Lucy may help resolve dependency metadata or download plans, but it must not bypass Stratum Artifact approval, staging, audit, or verification.

### Environment

An Environment is a template describing the required Minecraft runtime environment. It is not a running server.

An Environment may describe:

* Minecraft version
* Java version
* loader type
* loader version
* server core
* MCDR requirement
* Carpet requirement
* RuntimeProfile requirement
* base Artifacts
* default World Profile
* dependency or Lucy lock metadata
* notes

Environment references are validated at Room and Session creation boundaries. StratumMC does not auto-create or seed Environments unless explicitly requested through a service.

See [World Profile documentation](world-profile.md) for world configuration capture and restore workflows.

### World Manager

The World Manager will own world templates, world profiles, world cloning, reset, import, export, restore, and fork behavior.

A World Profile may describe:

* creative world
* void world
* normal world
* flat world
* seed
* generator settings
* datapack worldgen
* base world reference

The World Manager must prevent cross-session filesystem access. Base-world references should resolve to immutable or read-only data. Writable world state belongs to a specific Session workspace.

### Debug Mod / MC Bridge

The Debug Mod or MC Bridge is the future Minecraft-side integration layer. It exists because MCDR alone cannot provide strong access to Minecraft internal state.

The MC Bridge may eventually support:

* checkpoint prepare, commit, and abort
* world freeze or quiescence
* tick control
* scheduled tick inspection
* block event queue inspection
* chunk ticket inspection
* entity and block entity queries
* Carpet rule synchronization
* server internal state dumps
* controlled debugging mutations

The Debug Mod is powerful and dangerous. It must be protected by Permission Manager, Operation records, audit events, and Session isolation rules.

## Controller

The Controller composes repositories and services, exposes public API and CLI behavior, authorizes changes, schedules capacity, records audit events, and delegates machine-local work to Agents. It must not assume that Minecraft runs on the Controller host.

The Controller owns:

* Projects
* Rooms
* authoritative Session metadata
* Environment metadata
* Artifact metadata
* Operation records
* resource scheduling decisions
* permission decisions
* audit history
* Checkpoint metadata

The Controller does not directly:

* start Docker containers
* start MCDR
* start Minecraft
* copy world files
* inspect Minecraft internals
* execute Lucy provider logic
* write into an Agent runtime workspace

Agent observations do not directly change Controller records. Controller services compare observations against authoritative metadata and persist state transitions only through explicit Operations.

## Operation Coordination

The Operation Coordination layer sits between CLI/API requests and domain services. It provides:

* durable operation history
* per-session exclusion
* idempotent retries
* timeout classification
* request and operation correlation
* audit correlation
* failed-operation metadata

Compound operations such as start, stop, restart, checkpoint, restore, or fork must be represented as explicit Operations. Dangerous Operations should create or require a pre-operation Checkpoint according to policy.

See `operations.md`.

## Agent

An Agent owns a constrained session workspace and the outer runtime lifecycle on a runtime host.

An Agent may manage:

* runtime directories
* trusted child processes (including MCDR daemon)
* containers
* terminal stdin/stdout/stderr
* MCDR stdin/stdout/stderr streams
* runtime logs
* PID and exit-code observations
* resource observations
* Artifact materialization
* applied Artifact verification
* Environment materialization
* MCDR config and plugin materialization
* checkpoint packing
* bridge outbox/inbox/status manifests
* future sandbox setup

**Agent owns MCDR daemon supervision.** When a RuntimeProfile specifies MCDR execution, the Agent:

1. Creates MCDR runtime layout (`{sessionRoot}/mcdr/`)
2. Writes `config.yml` from Environment-specified MCDR config
3. Installs MCDR plugins from Artifact staging
4. Spawns MCDR as a child process with captured stdin/stdout/stderr
5. Writes commands to MCDR stdin on Controller request
6. Observes MCDR stdout for log patterns, readiness, and command feedback
7. Observes MCDR stderr for daemon errors
8. Detects MCDR crash or unexpected exit
9. Executes graceful stop sequence (stdin command → SIGTERM → SIGKILL)

The Controller depends only on the transport-independent `AgentClient` interface. Agent-local work is divided into runtime, process, log, file, resource, container, bridge, and checkpoint-worker interfaces. MCDR supervision is implemented inside `agent/mcdr/supervisor.go`.

An unexpected child exit (including MCDR crash) updates only Agent-local observations. It does not directly transition authoritative Session metadata. A later reconciliation phase must compare Agent observations with Controller state through explicit Operations and audit events.

For local multi-process development, `internal/agent/httptransport` exposes the Agent through an HTTP API. HTTP carries intent and observations only; lifecycle decisions and repository writes remain in the Controller.

See `docs/agent.md` and `docs/mcdr.md`.

## RuntimeProfile

RuntimeProfile describes how a Session is executed. Environment describes what the Session requires.

RuntimeProfile selection is part of the Agent protocol. The Controller may request an enabled profile ID and record that ID in Operation, Session, Checkpoint, and audit metadata. Profile definition and execution policy remain inside the trusted Agent deployment.

Known or planned RuntimeProfile families include:

* `dummy-process` — in-process Go goroutine with no OS process (test/dev only)
* `terminal` — direct `exec.Cmd` OS process (simple servers, direct Java)
* `mcdr-python` — Python-based MCDR daemon wrapper
* `mcdr-docker` — MCDR inside Docker container
* `minecraft-direct` — direct Java server without MCDR
* `external-runtime` — Agent-managed but externally defined execution

### MCDR profile example

A typical MCDR RuntimeProfile (`mcdr-python-1.17`) would define:

```yaml
id: mcdr-python-1.17
type: mcdr-python
command: python3
args: ["-m", "mcdreforged"]
workdir: "{sessionRoot}/mcdr"
env:
  MCDR_CONFIG: "{sessionRoot}/mcdr/config.yml"
  PYTHONUNBUFFERED: "1"
readinessCheck:
  type: log-pattern
  pattern: "MCDR started"
  timeout: 60s
healthCheck:
  type: process-alive
  maxSilentSeconds: 300
gracefulStop:
  - type: stdin-command
    command: "!!MCDR stop"
    timeout: 30s
  - type: signal
    signal: SIGTERM
    timeout: 10s
  - type: signal
    signal: SIGKILL
```

The Agent uses this profile to:

1. Materialize MCDR config and plugins
2. Launch `python3 -m mcdreforged` with captured I/O
3. Wait for readiness log pattern
4. Expose stdin for command sending
5. Parse stdout/stderr for log forwarding and observation
6. Execute graceful stop sequence on shutdown

Only the built-in `dummy-process` profile is registered by default. Trusted deployment wiring may register terminal or MCDR profiles. Controller requests can select only a profile ID; they cannot supply arbitrary commands, environment variables, or shell strings.

See `docs/runtime.md` and `docs/mcdr.md`.

## Container Runtime

Container Runtime is the future runtime isolation layer for real Minecraft/MCDR execution.

It should eventually manage:

* container create/start/stop/restart/kill
* container inspect
* logs
* controlled exec
* attach
* volume mounts
* resource limits
* network mode
* labels and metadata

Docker or another backend may implement this layer. MCDR must not own the container lifecycle. The Agent owns the outer lifecycle through a ContainerRuntime interface.

## MCDR Integration

MCDR is a daemon wrapper and plugin transport managed by the Agent as a supervised child process.

### Core principle

**Stratum Agent owns the MCDR daemon lifecycle. MCDR does not own the Stratum lifecycle.**

The Controller never calls MCDR directly. MCDR is started, stopped, and restarted by the Agent through a trusted RuntimeProfile. The Agent captures MCDR's stdin/stdout/stderr streams and observes its process state.

### Runtime model

MCDR runs as one of:

1. **OS process** — `python3 -m mcdreforged` launched by Agent via `exec.Cmd` or equivalent
2. **Container child** — future Docker/Podman container managed by Agent's ContainerRuntime

The Agent creates the MCDR runtime directory structure, writes `config.yml`, installs plugins, and materializes Environment-specified MCDR configuration before launching the daemon.

### I/O stream ownership

The Agent owns:

* **stdin** — write-only pipe to MCDR. Used to send console commands (`save-all`, `stop`, etc.) and plugin commands.
* **stdout** — read-only stream. Contains MCDR console output, Minecraft server logs, and plugin messages. Agent may buffer, forward to Controller observations, or stream to external log collectors.
* **stderr** — read-only stream. Contains MCDR daemon errors and Python exceptions. Agent logs these separately.

MCDR stdin is **not** exposed to end users. All commands go through Agent → MCDR stdin, never Controller → MCDR.

### Lifecycle integration

MCDR daemon lifecycle is controlled by the Agent through RuntimeProfile execution:

1. **Start** — Agent materializes MCDR config, installs plugins, spawns `python3 -m mcdreforged`, captures streams
2. **Stop** — Agent sends `!!MCDR stop` or `stop` to stdin, waits for graceful exit with timeout, force-kills if needed
3. **Restart** — Agent stops MCDR, waits for exit, starts fresh process
4. **Crash** — Agent observes unexpected exit code, records crash in runtime observation, does NOT auto-restart without Controller Operation

### Command sending

The Agent exposes `SendCommand(ctx, sessionID, command string)` to the Controller. Internally:

```text
Controller Operation → Agent.SendCommand(sessionID, "save-all flush")
    → Agent writes "save-all flush\n" to MCDR stdin
        → MCDR relays to Minecraft console
            → Agent observes stdout for success/failure log patterns
                → Agent returns success/error to Controller
```

Command sending is synchronous with timeout. The Agent may parse MCDR/Minecraft log output to detect command completion.

### Plugin integration

MCDR plugins (e.g., PrimeBackup, custom checkpoint plugins) can be used for **advisory backup workflows**, but they do not replace Stratum's authoritative checkpoint metadata or Agent-side world snapshot logic.

Plugin-triggered backups should:

1. Be invoked via `SendCommand(ctx, sessionID, "!!backup create <reason>")`
2. Write backup metadata to a known path inside the session runtime
3. Return a reference that the Agent can verify and register with the Controller

Plugins must not directly mutate Controller metadata, create Controller Operations, or bypass Agent supervision.

### What MCDR does NOT do

1. **MCDR does not prove strong world-state consistency.** `save-off` + `save-all flush` commands are advisory. Strong consistency requires the MC Bridge / Debug Mod layer.
2. **MCDR does not own checkpoint orchestration.** Checkpoint Operations originate from the Controller, coordinate through the Agent, and may optionally invoke MCDR plugin hooks.
3. **MCDR is not the primary start/stop mechanism.** The Agent's RuntimeProcess/ContainerRuntime is the source of truth for process state.
4. **MCDR does not replace the Agent.** MCDR is a child. The Agent supervises it.

### MCDR as a RuntimeProfile

MCDR execution is defined by a RuntimeProfile (e.g., `mcdr-python-1.17` or `mcdr-docker-managed`). The profile specifies:

* Command: `python3 -m mcdreforged`
* Working directory: `{sessionRoot}/mcdr`
* Environment variables: `MCDR_CONFIG={sessionRoot}/mcdr/config.yml`
* Readiness check: parse stdout for `MCDR started` log line
* Health check: process running + no recent stderr output
* Graceful stop: send `!!MCDR stop\n` to stdin, wait 30s, SIGTERM, wait 10s, SIGKILL

See `docs/mcdr.md` and `docs/runtime.md` for implementation details.

## MC Bridge

The MC Bridge is the Minecraft-side state and consistency bridge. It may be implemented by a future mod, plugin, or server-side protocol, depending on Minecraft version and loader.

It exists to support capabilities that MCDR cannot guarantee by itself:

* checkpoint prepare
* checkpoint commit
* checkpoint abort
* freeze or quiesce world state
* confirm save completion
* report bridge state
* expose selected debug information
* perform controlled internal-state inspection

The MC Bridge should be modeled separately from MCDR. MCDR may carry messages to or from an MC Bridge implementation, but it does not replace the MC Bridge.

## Checkpoint Orchestrator

Checkpoint orchestration coordinates Controller metadata, Agent runtime execution, optional MCDR command quiescence, optional MC Bridge consistency, and storage references.

A future strong checkpoint flow may look like:

```text
Controller creates checkpoint Operation (with requested consistency level)
    → Agent selects achievable consistency mode based on RuntimeProfile
        → If level >= command_quiesced: Agent sends save-off/save-all to MCDR stdin
            → Agent parses MCDR stdout for command completion
        → If level >= mc_bridge_prepared: Agent calls MC Bridge.Prepare()
            → MC Bridge freezes world state, returns PrepareToken
        → Agent or MCDR plugin snapshots world data
            → Agent verifies snapshot integrity
        → If MC Bridge was prepared: Agent calls MC Bridge.Commit(token) or Abort(token)
        → If MCDR command quiescence was used: Agent sends save-on to MCDR stdin
    → Agent returns snapshot reference and achieved consistency level
        → Controller records Checkpoint metadata with achieved level and audit
```

**MCDR's role in checkpoints:**

MCDR participates through:

1. **Command quiescence** (consistency level `command_quiesced`) — Agent sends `save-off`, `save-all flush` to MCDR stdin, waits for stdout confirmation, snapshots world, sends `save-on`. This is advisory; it does not guarantee full consistency.
2. **Plugin hooks** (consistency level `plugin_backup`) — Agent sends `!!backup create` or similar plugin command to MCDR stdin. MCDR plugin performs backup internally. Agent verifies backup metadata file written by plugin.

MCDR does **not**:

* Decide when to create checkpoints (Controller decides)
* Directly call Agent APIs (Agent calls MCDR via stdin)
* Guarantee strong consistency (MC Bridge is required for that)

The current implementation records metadata and may include Agent runtime-status snapshots. It does not yet claim to protect world data. Real checkpoint packing requires Agent-side world snapshot orchestration and storage backend integration.

## Managers and services

* **Project Manager** owns long-term membership and collaboration boundaries.
* **Room Manager** owns shared workspaces and their default shared Sessions.
* **Session Manager** owns explicit state transitions and lifecycle intent.
* **Resource Scheduler** evaluates global, project, user, type, and review limits.
* **World Manager** will create writable Session worlds from immutable base worlds and prevent cross-session filesystem access.
* **Checkpoint Manager** records semantic metadata and delegates snapshots and restores to Agent/storage backends.
* **Artifact Manager** hashes uploads, records compatibility and approval state, and prevents unapproved Artifacts from entering shared Sessions.
* **Permission Manager** enforces role, project, room, session-type, Artifact, checkpoint, and Debug Mod permissions.
* **Audit Log** records material mutations and dangerous operations.

Environment references are validated at Room and Session creation boundaries. Repositories remain storage-only; validation belongs at service, CLI, or API boundaries.

## Session lifecycle service

`internal/session/service` coordinates control-plane lifecycle operations. It implements prepare, start, stop, restart, freeze, unfreeze, crash marking, archive, and delete without launching a JVM directly from the Controller.

Before start or restart, the service derives current usage from persisted Sessions and asks the Resource Scheduler to enforce global, project, owner, type, and review limits. A denial leaves Session metadata unchanged.

When configured with an `AgentClient`, prepare/start/stop/restart/freeze calls are delegated after policy validation and before final metadata persistence. Agent failure leaves authoritative lifecycle state unchanged and is captured in Operation and audit metadata.

Freeze and unfreeze are protocol operations, but their meaning depends on the RuntimeProfile and bridge backend. A fake Agent may implement them as metadata-only observations. A future MC Bridge may implement them as world-state quiescence or checkpoint preparation.

Restart, archive, delete, checkpoint, restore, and fork operations contain hook points for future checkpoint/snapshot integration. They must not claim to protect world data unless a consistency level and storage reference prove it.

## Resource Scheduler

The Resource Scheduler determines whether an operation may consume runtime capacity.

It may evaluate:

* global limits
* project limits
* room limits
* user limits
* session-type limits
* review-session limits
* CPU, memory, disk, and runtime-host capacity
* TTL policies for fork/private/review Sessions

Typical scheduling decisions include:

* start shared Session
* create fork Session
* create review Session
* start private sandbox
* run checkpoint snapshot
* run Artifact review

The Resource Scheduler produces decisions. The Agent performs execution.

## Permission Manager

The Permission Manager enforces authorization at Project, Room, Session, Artifact, Checkpoint, and Debug Mod boundaries.

Example permissions include:

* `project.admin`
* `room.create`
* `room.configure`
* `session.start`
* `session.stop`
* `session.fork`
* `session.join`
* `checkpoint.create`
* `checkpoint.restore`
* `artifact.upload`
* `artifact.approve`
* `artifact.apply`
* `debug.use`
* `world.modify`

Shared Sessions require stricter permissions than fork, private, or review Sessions. Debug Mod and MC Bridge operations require especially strict permission checks and audit coverage.

## Lucy Integration

Lucy is integrated as a direct Go dependency under `github.com/mclucy/lucy` (local replace in `go.mod`). StratumMC calls Lucy's package resolution and artifact planning logic directly through Go function calls, not through external CLI invocation.

### Integration boundary

The Stratum-side Lucy boundary lives under `internal/integration/lucy`. It provides:

* `Adapter` interface — type-safe boundary for Environment and Artifact dependency resolution
* `EmbeddedAdapter` — production implementation that calls Lucy library functions directly
* `CLIAdapter` — fallback implementation that shells out to `lucy` command (backup only)
* `NoopAdapter` — no-op stub for tests or environments without Lucy

Production deployments should use `EmbeddedAdapter` to invoke Lucy's Go APIs directly within the Stratum process. This eliminates subprocess overhead, provides structured error handling, and allows tight integration with Stratum's Artifact and Environment workflows.

### Lucy responsibilities

Lucy provides:

* package reference parsing
* provider routing (e.g., Modrinth, CurseForge, Maven, URL)
* dependency closure resolution
* version conflict solving
* download metadata and checksums
* lock file generation
* cache-aware download planning

### Lucy non-responsibilities

Lucy must not:

* own Session lifecycle
* start or stop Minecraft servers
* manage MCDR daemons
* mutate Stratum Controller metadata
* perform Artifact approval or staging decisions

Stratum validates Artifact references and compatibility before delegating resolution to Lucy. Lucy returns dependency plans; Stratum Agent materializes them into session directories and records application results.

Stratum does not expose Lucy's internal Provider, package, or manifest types through its own stable interfaces. The `internal/integration/lucy` boundary translates between Lucy's resolution model and Stratum's Artifact and Environment domain.

See `docs/lucy.md`.

## Storage

Storage has two major categories.

### Metadata storage

Metadata storage persists:

* Projects
* Rooms
* Sessions
* Environments
* Artifacts
* Operations
* Audit events
* Checkpoints
* resource policies
* permissions

The current durable repository is in `internal/storage/filesystem`. It stores one atomic JSON file per project, room, session, checkpoint, artifact, environment, and resource policy. Audit events use append-only JSONL. The root is configurable and defaults to `.stratum/data` in the CLI.

Repository code validates IDs and required fields before constructing paths or writing files. Domain packages remain unaware of filesystem layout.

See `docs/storage.md`.

### Runtime and blob storage

Runtime and blob storage may contain:

* Artifact blobs
* materialized session Artifacts
* applied Artifact manifests
* world backups
* checkpoint payloads
* runtime manifests
* logs
* bridge event files
* temporary staging directories

Base-world references must resolve to immutable or read-only data. Checkpoint restore must operate only on the selected Session workspace.

## Audit log

Every material mutation should produce an immutable audit event containing actor, action, target, project context, timestamp, result, and selected metadata.

Audit coverage is required for:

* Project and Room changes
* Session lifecycle operations
* Session fork creation
* Artifact upload, approval, staging, application, and verification
* Checkpoint creation, restore, and deletion
* Resource scheduling denials
* Permission denials
* Debug Mod / MC Bridge operations
* Agent reconciliation decisions

The filesystem repository provides durable JSONL append and reload. Automatic service-level emission for every mutation is still TODO.

## Data flow

### Control-plane operation flow

```text
CLI / HTTP API
    -> Controller authorization
        -> Permission Manager
            -> Resource Scheduler
                -> Operation Coordination
                    -> domain service
                        -> repositories and audit log
                        -> AgentClient when machine-local work is needed
```

### Runtime execution flow

```text
Controller Operation
    -> Agent HTTP API
        -> RuntimeProfile
            -> process supervisor or ContainerRuntime
                -> optional MCDR daemon
                    -> Minecraft server process
                        -> optional MC Bridge / Debug Mod
        -> runtime observations
        -> Controller reconciliation and audit
```

### Checkpoint flow

```text
Controller checkpoint Operation
    -> Agent checkpoint worker
        -> choose consistency level
            -> optional MC Bridge prepare
                -> optional MCDR/plugin backup or Agent snapshot
                    -> verify storage reference
                        -> optional MC Bridge commit/abort
                            -> Controller records Checkpoint metadata
```

### Artifact flow

```text
Artifact import
    -> blob storage
        -> approval
            -> staging plan
                -> Agent materialization
                    -> apply plan
                        -> apply execute
                            -> verification
                                -> Session ready-for-start gate
```

## Non-goals

StratumMC should not:

* become a generic server panel
* let MCDR own Stratum lifecycle
* let Lucy own Session lifecycle
* let Controller directly mutate runtime files
* let Agent mutate authoritative Controller metadata
* treat filesystem backups as strong world-state checkpoints without consistency metadata
* allow Debug Mod operations without strict permissions and audit
* expose arbitrary shell execution from Controller requests

```
```
