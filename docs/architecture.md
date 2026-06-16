# Architecture

StratumMC is a Minecraft technical-testing control plane organized around long-lived Projects, shared Rooms, semantic Checkpoints, and isolated Sessions. It is not a generic server panel and does not treat a Minecraft server process as the top-level product object. The primary domain flow is:

```text
Project -> Room -> Session -> Checkpoint / Fork / Artifact / Environment
```

A Project represents a long-term collaboration unit such as a lab or engineering group. A Room represents a shared testing workspace inside a Project. A Session is an actual runnable server instance. Checkpoints capture semantic experiment snapshots that can later be restored, compared, or forked into isolated test sessions.

Domain packages contain policy and metadata only. Process execution, container management, filesystem mutation, MCDR daemon control, Minecraft-side bridge operations, and checkpoint packing live behind Agent-side integration interfaces.

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
* `command_quiesced`: command-level quiescence such as `save-off`, `save-all`, and `save-on`
* `plugin_backup`: backup produced by an MCDR or server-side backup plugin
* `mc_bridge_prepared`: Minecraft-side bridge confirmed a prepared checkpoint state

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
* trusted child processes
* containers
* terminal stdin/stdout/stderr
* runtime logs
* PID and exit-code observations
* resource observations
* Artifact materialization
* applied Artifact verification
* Environment materialization
* checkpoint packing
* bridge outbox/inbox/status manifests
* future sandbox setup

The Controller depends only on the transport-independent `AgentClient` interface. Agent-local work is divided into runtime, process, log, file, resource, container, bridge, and checkpoint-worker interfaces.

An unexpected child exit updates only Agent-local observations. It does not directly transition authoritative Session metadata. A later reconciliation phase must compare Agent observations with Controller state through explicit Operations and audit events.

For local multi-process development, `internal/agent/httptransport` exposes the Agent through an HTTP API. HTTP carries intent and observations only; lifecycle decisions and repository writes remain in the Controller.

See `docs/agent.md`.

## RuntimeProfile

RuntimeProfile describes how a Session is executed. Environment describes what the Session requires.

RuntimeProfile selection is part of the Agent protocol. The Controller may request an enabled profile ID and record that ID in Operation, Session, Checkpoint, and audit metadata. Profile definition and execution policy remain inside the trusted Agent deployment.

Known or planned RuntimeProfile families include:

* `dummy-process`
* `terminal`
* `mcdr-docker-managed`
* `minecraft-direct`
* `external-runtime`

Only the built-in `dummy-process` profile is registered by default. Trusted deployment wiring may register terminal or container-backed profiles. Controller requests can select only a profile ID; they cannot supply arbitrary commands, environment variables, or shell strings.

See `runtime.md`.

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

MCDR Integration is a daemon and plugin transport layer, not a clean embedded core API.

The Agent may run MCDR as a trusted child process or inside an Agent-managed container. MCDR may then manage Minecraft console and plugin behavior internally. Stratum should interact with it through constrained daemon IO and plugin bridge contracts.

MCDR Integration may support:

* daemon start/stop through RuntimeProfile
* stdin/stdout/stderr/log access
* console command sending
* plugin command relay
* request ID correlation
* plugin event feedback
* backup plugin triggering
* PrimeBackup-like workflows

MCDR is not the top-level Stratum lifecycle controller. The Controller must not use MCDR directly for primary start, stop, restart, checkpoint, or restore behavior.

MCDR also does not prove strong world-state consistency. Strong checkpoint preparation belongs to the MC Bridge / Debug Mod layer.

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

Checkpoint orchestration coordinates Controller metadata, Agent runtime execution, optional MCDR plugin backups, optional MC Bridge consistency, and storage references.

A future strong checkpoint flow may look like:

```text
Controller creates checkpoint Operation
    -> Agent selects consistency mode
        -> MC Bridge prepare if required
            -> Agent or plugin snapshots world data
                -> Agent verifies snapshot
                    -> MC Bridge commit or abort
                        -> Controller records Checkpoint metadata and audit
```

The current implementation records metadata and may include Agent runtime-status snapshots. It does not yet claim to protect world data.

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

Lucy Integration is a dependency and package planning integration. It must not own Session lifecycle.

Lucy may eventually provide:

* package reference parsing
* provider routing
* dependency closure resolution
* version conflict solving
* download metadata
* lock generation
* cache-aware download planning

Stratum must not depend on premature Lucy public APIs. Any future embedded interface should be grounded in Lucy's real install pipeline and type model. Stratum should not expose Lucy internal Provider, package, or manifest types through its own stable interfaces.

The Stratum-side Lucy boundary lives under `internal/integration/lucy`. The current `NoopAdapter` performs no I/O, resolution, downloads, or manifest processing.

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
