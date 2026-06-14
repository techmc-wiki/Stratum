# Operation Coordination

Every session lifecycle action is represented by a durable Operation record before execution begins. Records are stored under `.stratum/data/operations/<operation-id>.json` and move through `pending`, `running`, and one terminal status.

An operation records the actor, request ID, optional idempotency key, target session, previous and intended state, final state, agent metadata, result, and structured failure details. Session audit events carry `operationId` and `requestId`; separate operation audit events record creation, start, completion, and timeout.

Only one `pending` or `running` operation may target a session at a time within a controller process. A conflicting request returns the active operation ID. This is intentionally not a distributed lock; production multi-controller coordination remains future work.

The tuple `(actor, action, session, idempotency key)` identifies a retried request. Reusing it returns the existing operation without invoking the agent or mutating session state again.

Lifecycle commands accept `--idempotency-key`, `--request-id`, and `--operation-timeout`. Use `stratum operations list [--session ID] [--status STATUS]` and `stratum operations inspect --id ID` to inspect history.

A timeout marks the operation `timed_out`; session metadata is changed only after the underlying lifecycle action succeeds.

Runtime observation persists diagnostic RuntimeObservation records and audit
events, but does not create an Operation or mutate Session state.

Artifact staging plan creation writes audit events but does not create a
lifecycle Operation. It is metadata-only validation of staging intent and does
not call the Agent or mutate runtime directories.

Planned staging records are written only after the linked blob passes SHA-256
verification. Verification failures follow the existing rejected-plan pattern:
the rejected plan and audit metadata include the Artifact, Session, payload
hash, verification status, and rejection reason. Artifact metadata is not
changed.

Successful Agent materialization writes `artifact.materialized` audit metadata
including Session, Artifact, staging plan, actor, payload hash and size, target,
runtime-relative path, Agent identity/mode, and idempotency result. It does not
create a lifecycle Operation.

Artifact approval and rejection also write audit events without creating
Operations. They are metadata review actions and do not copy, mount, install, or
execute artifact payloads.

Approval writes `artifact.approved` only after the linked blob passes SHA-256
verification. A failed verification leaves Artifact and review metadata
unchanged and, consistently with current service failure handling, does not
append a separate failed approval audit event.

Artifact metadata creation writes `artifact.created` with artifact, project,
actor, type, and pending status metadata. It does not create an Operation or
accept an artifact payload.

Artifact payload import writes `artifact.payload.imported` with the artifact,
actor, recomputed SHA-256 algorithm/hash, and payload size. It does not create
an Operation, approve the Artifact, or copy payload content into a runtime.

## Pre-start Artifact Readiness Metadata

Remote `session.start` Operations run a read-only artifact gate before Agent
prepare/start. Operation and lifecycle audit metadata include
`artifactCheckEnabled`, `stagingReadinessStatus`, `appliedVerifyStatus`,
`totalApplied`, `validApplied`, `missingApplied`, `corruptedApplied`, and
`artifactReadinessIssues`. A blocked gate completes the Operation as failed and
leaves Session state unchanged.

The gate does not materialize, apply, repair, delete, or execute artifacts and
does not create checkpoint backups.

## Environment Materialization Metadata

Session start and restart Operations call Agent Environment materialization after
Environment/RuntimeProfile compatibility validation and before Agent runtime
launch. Operation metadata records:

- `environmentMaterializationStatus`: preparation status returned by the Agent
  (e.g., `prepared`, `failed`).
- `environmentMaterializationDirectories`: count of created directories.
- `environmentMaterializationManifest`: absolute path to the materialization
  manifest file (e.g., `runtime-root/sessions/<session-id>/config/environment-materialization.json`).
- `environmentMaterializationError`: error message if materialization failed.

If materialization fails, the Operation completes as failed, and Session state
remains unchanged (e.g., `created` or `stopped`). Lifecycle audit metadata also
includes Environment ID, RuntimeProfile ID, and compatibility validation results.

Environment materialization does not install Java, Minecraft, Fabric, or Carpet,
does not download files, does not call Lucy, and does not start MCDR or
Minecraft. It prepares runtime directory structure and writes an informational
manifest at `config/environment-materialization.json`.

## Manual Reconciliation Operations

Reconciliation is an explicit human-confirmed metadata repair. Supported
Controller metadata actions are:

```powershell
stratum sessions reconcile mark-stopped --id SESSION --actor ACTOR --reason "REASON"
stratum sessions reconcile mark-crashed --id SESSION --actor ACTOR --reason "REASON"
```

`mark-stopped` creates a `session.reconcile.mark-stopped` Operation and audit
events, then updates an eligible Controller Session to `stopped`. The reason and
any available RuntimeObservation classification are recorded. Running, crashed,
frozen, starting, and stopping Sessions are eligible; already stopped or other
inactive states are rejected with a failed Operation.

`mark-crashed` creates a `session.reconcile.mark-crashed` Operation and audit
events, then updates an eligible Controller Session to `crashed`. Running,
starting, stopping, and frozen Sessions are eligible. Created, preparing,
stopped, crashed, archived, and deleted Sessions are rejected with a failed
Operation. If an Agent URL is supplied, the command attempts to persist a fresh
RuntimeObservation and attach its metadata. An unreachable Agent does not block
the Controller-only repair.

These actions only change Controller metadata. They do not stop, kill, restart,
or otherwise mutate an Agent runtime, and they do not create checkpoints. Runtime
process control remains a separate Agent operation. Automatic reconciliation is
future work.

## Manual Runtime Stop Reconciliation

`stop-runtime` is the separate, human-confirmed Agent action:

```powershell
stratum --agent-url http://127.0.0.1:8787 sessions reconcile stop-runtime --id SESSION --actor ACTOR --reason "REASON"
```

It inspects the Agent runtime, records the observation, then calls the existing
Agent stop operation. The Controller Session state is not changed. Operators
may use `mark-stopped` and `stop-runtime` in sequence when metadata and runtime
state have diverged.

An unreachable Agent or failed stop produces a failed Operation and audit
record without changing Controller Session state. This command does not create
checkpoints, add new kill mechanisms, or perform automatic reconciliation.
