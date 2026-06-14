# Architecture

StratumMC is a control plane organized around Projects and shared Rooms. Domain
packages contain policy and metadata only; process execution and filesystem
operations live behind integration and agent interfaces.

The Operation Coordination layer sits between CLI/API requests and session
services. It provides durable history, per-session exclusion, idempotent
retries, timeout classification, and audit correlation. See `operations.md`.

The Agent runtime layer includes dummy and managed terminal executors. A
long-running Agent owns runtime handles, terminal observations, and logs. It
reports those observations through the session-oriented Agent protocol but has
no access to Controller repositories. Terminal processes use direct argv-based
`os/exec` without a shell. See `runtime.md` for the RuntimeProfile boundary.

RuntimeProfile selection is now part of the Agent protocol. The Controller may
request an enabled profile ID, records that ID in Operation and audit metadata,
and leaves profile definition and execution policy inside the trusted Agent.
Only the built-in `dummy-process` profile is registered by default. Trusted
deployment wiring may register terminal profiles; Controller requests can
select only an ID and cannot supply commands or environment variables.

## Controller

The controller composes repositories and services, exposes the public API/CLI,
authorizes changes, schedules capacity, records audit events, and delegates
machine-local work to agents. It must not assume that Minecraft runs on the
controller host.

The Controller owns Projects, Rooms, authoritative Session metadata, Operation
records, resource scheduling, permissions, audit history, checkpoint metadata,
and artifact metadata. Agent observations do not directly change these records;
controller services decide whether and when to persist a state transition.

## Agent

An agent owns a constrained session workspace and the outer runtime lifecycle on
a runtime host. It starts, stops, restarts, and eventually force-terminates
trusted runtime processes; owns terminal stdin/stdout/stderr and runtime logs;
observes PID, exit code, status, crashes, and resource usage; and will later
perform filesystem operations, checkpoint packing, and sandbox setup.

The controller depends only on the transport-independent `AgentClient`
interface. Agent-local work is divided into runtime, process, log, file,
resource, and checkpoint-worker interfaces. The current local fake implements
the protocol without touching a JVM or filesystem. See `docs/agent.md`.

For local multi-process development, `internal/agent/httptransport` exposes the
Agent through an HTTP API. The in-process CLI path uses a deterministic fake;
the long-running development Agent uses the dummy process supervisor. HTTP
carries intent and observations only; lifecycle decisions and repository writes
remain in the Controller.

Unexpected child exit updates only Agent-local observations. It does not
directly transition authoritative Session metadata; a later reconciliation
phase must compare Agent observations with Controller state through explicit
Operations and audit events.

## Managers and services

- **Project Manager** owns long-term membership and collaboration boundaries.
- **Room Manager** owns shared workspaces and their default shared sessions.
  Room creation validates that the referenced Environment exists before persisting
  Room metadata.
- **Session Manager** owns explicit state transitions and session lifecycle.
  Session creation validates that the referenced Environment exists before
  persisting Session metadata. Sessions inherit EnvironmentID from their Room
  when applicable.
- **Resource Scheduler** evaluates global, project, user, type, and review limits.
- **World Manager** will create writable session worlds from immutable base
  worlds and prevent cross-session filesystem access.
- **Checkpoint Manager** records semantic metadata and delegates snapshots and
  restores to the storage backend. Dangerous operations must first create a
  pre-operation checkpoint.
- **Artifact Manager** hashes uploads, records compatibility and approval state,
  and prevents unapproved artifacts from entering shared sessions.
- **Permission Manager** enforces role and session-type rules. Shared rooms have
  stricter requirements than fork, private, or review sessions.

Environment references are validated at Room and Session creation boundaries.
Repositories remain storage-only; validation belongs at service/CLI/API
boundaries. StratumMC does not auto-create or seed Environments.

## Session lifecycle service

`internal/service/sessionsvc` coordinates control-plane lifecycle operations. It
implements prepare, start, stop, restart, freeze, unfreeze, crash marking,
archive, and delete without launching a JVM. Compound operations use only the
explicit domain transitions; for example, starting a new session advances
`created -> preparing -> starting -> running` and stopping a running session
advances `running -> stopping -> stopped`.

Before start or restart, the service derives current usage from persisted
sessions and asks the Resource Scheduler to enforce global, project, owner, and
review limits. A denial leaves session metadata unchanged. Every successful or
failed operation appends an audit event with previous state, intended next
state, result, and any reason.

When configured with an `AgentClient`, prepare/start/stop/restart/freeze calls
are delegated after policy validation and before final metadata persistence.
Agent failure leaves the lifecycle state unchanged and is captured in audit
metadata.

Freeze and unfreeze are distinct protocol operations even though the fake agent
implements them as metadata-only observations. This preserves a clear contract
for future runtime pause semantics.

Restart, archive, delete, and crash operations contain named TODO hook points
for future checkpoint/snapshot integration. They do not claim to protect world
data yet.

## Integrations

**MCDR Integration** is a future optional RuntimeProfile and server-side bridge.
The Agent may launch MCDR as a trusted child runtime; MCDR may then manage
Minecraft console/plugin behavior internally. MCDR is not the top-level Stratum
lifecycle controller, and the Controller must not use it directly for primary
start, stop, or restart behavior.

**Lucy Integration** resolves manifests, verifies environments, and reports lock
hashes. Its interface intentionally contains no JVM or session control methods.

**Storage Backend** owns object I/O and world snapshots. Base-world references
must resolve to immutable/read-only data. Checkpoint restore must operate only on
the selected session workspace.

## Metadata repositories

The current durable repository is in `internal/repository/filesystem`. It stores
one atomic JSON file per project, room, session, checkpoint, artifact,
environment, and resource policy. Audit events use append-only JSONL. The root
is configurable and defaults to `.stratum/data` in the CLI.

Repository code validates IDs and required fields before constructing paths or
writing files. Domain packages remain unaware of filesystem layout. See
`docs/storage.md` for the record layout and write guarantees.

## Audit log

Every material mutation should produce an immutable audit event containing
actor, action, target, project context, timestamp, and selected metadata. The
filesystem repository now provides durable JSONL append and reload. Automatic
service-level emission for every mutation is still TODO.

## Data flow

```text
CLI / HTTP API
    -> Controller authorization and services
        -> resource scheduler
        -> repositories and audit log
        -> Agent HTTP API
            -> terminal / process runtime supervisor
                -> trusted child process
                    -> optional future MCDR RuntimeProfile
                        -> Minecraft server process
            -> future storage/checkpoint packing
            -> future Lucy environment verification
```

Repository implementations may be in-memory or file-backed. Domain models never
derive identity or authorization from deployment-specific paths.
