# Architecture

StratumMC is a control plane organized around Projects and shared Rooms. Domain
packages contain policy and metadata only; process execution and filesystem
operations live behind integration and agent interfaces.

## Controller

The controller composes repositories and services, exposes the public API/CLI,
authorizes changes, schedules capacity, records audit events, and delegates
machine-local work to agents. It must not assume that Minecraft runs on the
controller host.

## Agent

An agent owns a constrained session workspace on a runtime host. Future agent
implementations will prepare files, invoke the MCDR bridge, report capacity, and
perform checkpoint I/O. Controller-to-agent authentication and process
supervision remain TODO.

## Managers and services

- **Project Manager** owns long-term membership and collaboration boundaries.
- **Room Manager** owns shared workspaces and their default shared sessions.
- **Session Manager** owns explicit state transitions and session lifecycle.
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

Restart, archive, delete, and crash operations contain named TODO hook points
for future checkpoint/snapshot integration. They do not claim to protect world
data yet.

## Integrations

**MCDR Integration** is the runtime bridge for start, stop, restart, commands,
and logs. The interface is in `internal/integration/mcdr`.

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
    -> authorization and services
    -> resource scheduler
    -> repositories and audit log
    -> runtime agent
        -> MCDR (live JVM control)
        -> storage (world/checkpoint data)
        -> Lucy (environment verification only)
```

Repository implementations may be in-memory or file-backed. Domain models never
derive identity or authorization from deployment-specific paths.
