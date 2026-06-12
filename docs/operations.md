# Operation Coordination

Every session lifecycle action is represented by a durable Operation record before execution begins. Records are stored under `.stratum/data/operations/<operation-id>.json` and move through `pending`, `running`, and one terminal status.

An operation records the actor, request ID, optional idempotency key, target session, previous and intended state, final state, agent metadata, result, and structured failure details. Session audit events carry `operationId` and `requestId`; separate operation audit events record creation, start, completion, and timeout.

Only one `pending` or `running` operation may target a session at a time within a controller process. A conflicting request returns the active operation ID. This is intentionally not a distributed lock; production multi-controller coordination remains future work.

The tuple `(actor, action, session, idempotency key)` identifies a retried request. Reusing it returns the existing operation without invoking the agent or mutating session state again.

Lifecycle commands accept `--idempotency-key`, `--request-id`, and `--operation-timeout`. Use `stratum operations list [--session ID] [--status STATUS]` and `stratum operations inspect --id ID` to inspect history.

A timeout marks the operation `timed_out`; session metadata is changed only after the underlying lifecycle action succeeds.

Runtime observation is read-only and does not create an Operation.

## Manual Reconciliation Operations

Reconciliation is an explicit human-confirmed metadata repair. The first
supported action is:

```powershell
stratum sessions reconcile mark-stopped --id SESSION --actor ACTOR --reason "REASON"
```

It creates a `session.reconcile.mark-stopped` Operation and audit events, then
updates an eligible Controller Session to `stopped`. The reason and any
available RuntimeObservation classification are recorded. Running, crashed,
frozen, starting, and stopping Sessions are eligible; already stopped or other
inactive states are rejected with a failed Operation.

This action only changes Controller metadata. It does not stop, kill, restart,
or otherwise mutate an Agent runtime. Runtime process control remains a
separate Agent operation. Automatic reconciliation is future work.
