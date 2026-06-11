The durable metadata phase is complete and `go test -count=1 ./...` passes.

Manual CLI smoke testing mostly works:
- projects create/list works
- rooms create/list works
- sessions create/list works
- checkpoints create works

However, `checkpoints list` currently returns:
unknown command "checkpoints list"

Please first fix the missing checkpoint CLI list command, then implement the next phase: Session Lifecycle Service.

Goals:
1. Keep the existing architecture and AGENTS.md rules.
2. Do not implement real Minecraft, MCDR, Lucy, or JVM process launching yet.
3. Implement control-plane session lifecycle logic using existing domain models and repositories.
4. Session lifecycle operations must be resource-policy-aware and audit-logged.
5. Preserve the existing file-backed metadata storage and global `--data-dir` behavior.

Small CLI fix first:
- Add `stratum checkpoints list`.
- If practical, also add `stratum checkpoints get --id <checkpoint_id>`.
- The list command should read from the file-backed checkpoint repository.
- Keep output simple and consistent with projects/rooms/sessions list.

Required session lifecycle operations:
- prepare
- start
- stop
- restart
- freeze
- unfreeze
- mark-crashed
- archive
- delete, if already stopped/archived and allowed

Session state rules:
- created -> preparing
- preparing -> starting
- starting -> running
- running -> stopping
- stopping -> stopped
- running -> frozen
- frozen -> running
- running -> crashed
- starting -> crashed
- stopped -> starting
- stopped -> archived
- archived -> deleted
- crashed -> stopped
- crashed -> archived

If the existing state machine differs, update it carefully and add tests.

Resource policy:
- Before starting a session, check Resource Scheduler.
- Enforce global max running sessions.
- Enforce per-project running session limits.
- Enforce per-user private/fork session limits if the data model supports owner/creator.
- Return a structured denial reason if a session cannot start.
- Do not silently mutate state when resource policy denies start.

Audit logging:
- Every lifecycle operation should append an audit event.
- Audit event should include:
  - actor
  - action
  - target session id
  - previous state
  - next state
  - result: success/failure
  - denial/error reason if any
  - timestamp

Checkpoint hook stubs:
- For restart, archive, delete, and mark-crashed, add TODO hook points for future pre-operation checkpoint or crash snapshot.
- Do not implement real checkpoint filesystem backup yet.

CLI updates:
Add or update commands:
- stratum sessions prepare --id <session_id> --actor <actor>
- stratum sessions start --id <session_id> --actor <actor>
- stratum sessions stop --id <session_id> --actor <actor>
- stratum sessions restart --id <session_id> --actor <actor>
- stratum sessions freeze --id <session_id> --actor <actor>
- stratum sessions unfreeze --id <session_id> --actor <actor>
- stratum sessions mark-crashed --id <session_id> --actor <actor> --reason <reason>
- stratum sessions archive --id <session_id> --actor <actor>
- stratum sessions delete --id <session_id> --actor <actor>

The CLI should use the file-backed repositories and the global --data-dir flag.

Tests:
Add tests for:
- checkpoint list CLI/repository behavior
- valid lifecycle transitions
- invalid lifecycle transitions
- start denied by resource policy
- start success when resource policy allows
- audit event written on success
- audit event written on failure
- session state persists after lifecycle operation
- restart behavior
- freeze/unfreeze behavior
- crashed session handling

Docs:
- Update docs/architecture.md with Session Lifecycle Service.
- Update docs/mvp.md to mark lifecycle control as the current phase.
- Update docs/storage.md if audit event shape or state persistence changes.

Verification:
- Run gofmt.
- Run go test -count=1 ./...
- Run CLI smoke tests:
  - go run ./cmd/stratum --data-dir .stratum/data checkpoints list
  - go run ./cmd/stratum --data-dir .stratum/data sessions start --id demo-session --actor bryan
  - go run ./cmd/stratum --data-dir .stratum/data sessions list
  - go run ./cmd/stratum --data-dir .stratum/data sessions stop --id demo-session --actor bryan
  - go run ./cmd/stratum --data-dir .stratum/data sessions freeze --id demo-session --actor bryan
  - go run ./cmd/stratum --data-dir .stratum/data sessions unfreeze --id demo-session --actor bryan
  - go run ./cmd/stratum --data-dir .stratum/data sessions mark-crashed --id demo-session --actor bryan --reason "manual test"
  - go run ./cmd/stratum --data-dir .stratum/data sessions archive --id demo-session --actor bryan

After finishing, summarize changed files, design choices, test results, and remaining TODOs.