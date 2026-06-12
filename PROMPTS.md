Implement the next atomic task:

lifecycle: add explicit manual reconciliation operation for mark-stopped

Context:
- RuntimeObservation contract is complete.
- `sessions observe` can classify mismatches without mutating Controller state.
- Controller remains source of truth for Session metadata/state.
- Agent owns runtime/process observations.
- Observations do not automatically repair state.
- We now need one explicit, audited, manual reconciliation action.

Keep this task atomic.

Goals:
1. Add a manual reconciliation operation for marking a Controller Session as stopped.
2. Do not implement automatic reconciliation.
3. Do not implement MCDR.
4. Do not implement Minecraft launching.
5. Do not implement Lucy integration.
6. Do not implement checkpoint backup.
7. Do not add broad reconciliation framework beyond what is needed for this single action.
8. Do not modify unrelated docs or architecture rules.

Required behavior:
Add an explicit command:

`stratum sessions reconcile mark-stopped --id <session_id> --actor <actor> --reason <reason>`

This command should:
- load the Controller Session metadata,
- optionally use Agent observation if `--agent-url` is provided,
- create an Operation record,
- write audit events,
- transition the Controller Session to `stopped` only if the current state makes sense,
- require an actor,
- require a non-empty reason,
- not stop any runtime process,
- not call Agent stop,
- not mutate Agent runtime state.

Allowed source states:
- running
- crashed
- frozen
- starting
- stopping

Disallowed source states:
- created
- preparing
- stopped
- archived
- deleted

If disallowed:
- return structured error,
- create failed Operation or at least audit the failed attempt according to existing operation pattern,
- do not mutate Session state.

Observation integration:
If `--agent-url` is provided:
- call existing Agent inspect / observation path,
- include observation mismatch type, severity, and recommended action in Operation metadata and audit metadata.
- If observation says no mismatch, still allow mark-stopped only if user explicitly provided reason, but include warning metadata if practical.
- Do not require Agent to be reachable. If Agent is unreachable, allow a flag if needed:
  - either `--allow-without-agent`
  - or document that mark-stopped can run without agent because it is Controller-only.
Prefer simple behavior: mark-stopped can run without Agent, but records whether observation was available.

Operation:
Use the existing Operation Coordination Layer.

Action name:
- `session.reconcile.mark-stopped`

Operation metadata should include:
- reason
- previousState
- nextState
- observationAvailable
- mismatchType, if available
- severity, if available
- recommendedAction, if available

Audit:
Write audit events for:
- operation created/started/succeeded/failed, if this is the existing pattern.
- session reconciliation result.

Audit metadata should include:
- operationId
- requestId
- actor
- reason
- previousState
- nextState
- observation/mismatch info if available.

Session state:
- On success, persist Session state as `stopped`.
- Update last runtime/agent message if such fields exist, e.g. “manually reconciled as stopped”.
- Do not change assigned agent unless existing design requires it.
- Do not clear runtime profile metadata unless clearly correct.
- Do not delete operations.

CLI:
Add:
- `stratum sessions reconcile mark-stopped --id <session_id> --actor <actor> --reason <reason>`

Optional:
- support `--request-id`
- support `--idempotency-key`

If existing lifecycle commands already support request/idempotency flags, reuse the same pattern.

Tests:
Add focused tests for:
- running -> stopped succeeds.
- crashed -> stopped succeeds.
- frozen -> stopped succeeds.
- stopped -> stopped fails or is no-op according to chosen behavior; prefer fail with clear message.
- archived/deleted cannot be reconciled.
- reason is required.
- actor is required.
- Operation is created.
- Audit includes operationId and reason.
- Session state persists as stopped after success.
- Failed reconciliation does not mutate Session state.
- Observation metadata is included when available, if easy.
- Existing lifecycle/operation/observation tests still pass.

Documentation:
Update narrowly:
- docs/operations.md
- docs/runtime.md or docs/agent.md if needed
- docs/mvp.md if current phase list is tracked

Add a short section:
"Manual Reconciliation Operations"

It should say:
- Observations only detect mismatch.
- Reconciliation operations are explicit human-confirmed repairs.
- mark-stopped only updates Controller metadata.
- It does not stop or kill runtime processes.
- Runtime stop/kill remains a separate Agent operation.
- Automatic reconciliation is future work.

Important non-goals:
- Do not implement automatic repair.
- Do not implement mark-crashed reconciliation yet.
- Do not implement stop-runtime reconciliation yet.
- Do not add MCDR.
- Do not add Minecraft.
- Do not add Lucy.
- Do not implement checkpoint backup.
- Do not implement Web UI.

Verification:
- Run gofmt.
- Run go test -count=1 ./...
- Run git diff --check.

Manual smoke test:

Use clean data dir:
$data=".stratum/reconcile-test"
Remove-Item -Recurse -Force $data -ErrorAction SilentlyContinue

Terminal 1:
go run ./cmd/stratum-agent serve --listen 127.0.0.1:8787

Terminal 2:
go run ./cmd/stratum --data-dir $data projects create --id demo --name "Demo Project"
go run ./cmd/stratum --data-dir $data rooms create --id demo-room --project demo --name "Demo Room"
go run ./cmd/stratum --data-dir $data sessions create --id demo-session --project demo --room demo-room --type shared

go run ./cmd/stratum --data-dir $data --agent-url http://127.0.0.1:8787 sessions prepare --id demo-session --actor bryan
go run ./cmd/stratum --data-dir $data --agent-url http://127.0.0.1:8787 sessions start --id demo-session --actor bryan --runtime-profile dummy-process

go run ./cmd/stratum --data-dir $data --agent-url http://127.0.0.1:8787 sessions observe --id demo-session

go run ./cmd/stratum --data-dir $data sessions reconcile mark-stopped --id demo-session --actor bryan --reason "manual reconciliation smoke test"

go run ./cmd/stratum --data-dir $data sessions list
go run ./cmd/stratum --data-dir $data operations list --session demo-session

Inspect:
.stratum/reconcile-test/audit/events.jsonl

Final response format:
Follow AGENTS.md Atomic Change / Commit Policy.

Report:
1. Verification
2. Atomic commit summary
3. Behavior changes
4. Remaining TODOs
5. Suggested next atomic task