# Metadata Storage

StratumMC's current durable repository stores control-plane metadata beneath a
configurable data directory. The CLI defaults to `.stratum/data`; deployments
should provide an explicit location with `--data-dir`.

Operation metadata is retained as one atomic JSON document per operation in
`operations/`, using the same temporary-file-and-rename path as other entities.

## Layout

```text
<data-dir>/
  projects/<project-id>.json
  rooms/<room-id>.json
  sessions/<session-id>.json
  runtime-observations/<observation-id>.json
  checkpoints/<checkpoint-id>.json
  artifacts/<artifact-id>.json
  environments/<environment-id>.json
  resource-policies/<policy-id>.json
  audit/events.jsonl
```

Paths are a repository concern. Domain objects contain IDs and explicit
references, never deployment-specific metadata paths. IDs are restricted to
ASCII letters, numbers, `.`, `_`, and `-` so an ID cannot escape its entity
directory.

## JSON records

Projects, rooms, sessions, checkpoints, artifacts, environments, and resource
policies use one JSON document per object. Runtime observations are append-only
diagnostic records with create/get/list behavior and optional list-by-session
filtering. Create operations reject an existing ID. Updates require an existing
object, and deletes/get operations return typed not-found errors.

Lists read every `.json` record and return records ordered by filename. A
malformed or unknown-field record stops the list with an actionable repository
error rather than silently dropping metadata.

## Atomic writes

JSON metadata is encoded to a temporary file in the destination directory. The
repository syncs and closes the temporary file before renaming it over the final
path. If encoding or syncing fails, the temporary file is removed and an
existing destination remains unchanged.

This protects individual records from partial writes. Cross-record operations
are not transactional yet; callers must not assume that creating metadata and
an audit event is one atomic unit.

## Audit log

Audit events are append-only JSON Lines in `audit/events.jsonl`. Each event is
encoded on one line, flushed with `fsync`, and read in append order. The current
mutex coordinates writers sharing one repository process. Cross-process file
locking and log rotation remain TODO before multi-controller deployment.

Session lifecycle events use actions such as `session.start` and
`session.freeze`. Their metadata contains `previousState`, `nextState`, and
`result` (`success` or `failure`). Failed operations also include `reason`;
successful crash marking records the supplied crash reason in the same field.

Session state is written atomically before a success audit is appended. These
two files are not a transaction: an audit append failure can be reported after
the state file was updated. A future transactional event/outbox design should
close that gap before multi-controller operation.

Session JSON may also contain `assignedAgentId`, `lastAgentStatus`,
`lastRuntimeMessage`, and `runtimeEndpoint`. These are control-plane
observations and routing metadata; they are not proof that a real process
exists. Agent-backed lifecycle audit events add `agentId`, `agentResult`, and
`agentMessage`.

## Scope

This repository stores metadata only. It does not store uploaded artifact
payloads, live session files, base worlds, checkpoint world snapshots, secrets,
or MCDR/Lucy runtime state. Those remain behind separate storage and runtime
interfaces.
