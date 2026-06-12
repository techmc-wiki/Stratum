# MVP Scope

The MVP establishes a small, testable control-plane core without launching a
Minecraft server.

## Completed phases

- Go repository skeleton and core domain models.
- File-backed metadata storage and append-only audit history.
- Session lifecycle service and resource-policy enforcement.
- HTTP Controller-Agent transport.
- Durable Operation coordination, idempotency, locking, and timeout records.
- Agent Process Supervision Stub.
- Trusted RuntimeProfile model, validation, built-in registry, HTTP discovery,
  and dummy profile selection.
- Managed terminal executor with argv-only launch, constrained working
  directories, bounded stdout/stderr logs, stop strategies, and exit tracking.
- Trusted local RuntimeProfile JSON loading with strict validation.
- Read-only RuntimeObservation classification and `sessions observe` CLI output.
- Persisted RuntimeObservation records with list/inspect CLI and diagnostic audit
  events.
- Audited manual `mark-stopped` Controller metadata reconciliation.
- Audited manual `stop-runtime` Agent runtime reconciliation.
- Audited manual `mark-crashed` Controller metadata reconciliation.

The current HTTP Agent maintains safe cross-platform dummy runtimes, captures
lifecycle logs, reports running/stopped process observations, and counts active
runtimes in resource reports. Lifecycle decisions remain in the Controller. No
command starts Minecraft, MCDR, Lucy, or another JVM process.

## Next phase: Additional Explicit Reconciliation

RuntimeProfile loading, runtime mismatch detection, persisted observation
history, and explicit mark-stopped, stop-runtime, and mark-crashed reconciliation
are implemented. The next phase may add another narrowly scoped, authorized
reconcile Operation. It must not perform automatic repair or accept arbitrary
user-supplied commands.

A later phase may add **MCDR RuntimeProfile v0**, where the Agent launches MCDR
as a trusted child process. MCDR may manage Minecraft internally, but it will not
replace Agent process supervision or become the Controller's lifecycle manager.

## Included

- Project, Room, Session, Checkpoint, Artifact, Environment, ResourcePolicy,
  AuditEvent, and Operation domain models.
- Explicit session state transitions and resource-policy decisions.
- SHA-256 artifact hashing and pending-by-default metadata.
- Checkpoint metadata construction plus list/rollback service stubs.
- A Minecraft 1.17 Fabric + MCDR + Carpet environment template.
- In-memory and durable file-backed metadata repositories.
- Atomic JSON persistence and append-only JSONL audit persistence.
- Persistent CLI create/list and lifecycle flows.
- Durable lifecycle Operation records with request correlation.
- Deterministic local fake Agent for focused tests.
- Long-running HTTP Agent with Go-native dummy process supervision.
- Runtime inspect, logs, resource reports, and structured failure injection.
- Standard-library Agent HTTP server and HTTP AgentClient.
- Explicit transport DTOs, bounded JSON decoding, and client timeouts.
- Optional shared bearer token and request-ID propagation.
- MCDR, Lucy, storage, and runtime-agent interface stubs.
- Standard-library tests for core behavior.

## Deferred

- Additional explicit reconciliation actions.
- MCDR RuntimeProfile v0 and real Minecraft process integration.
- Production Agent authentication, TLS policy, retries, and reconciliation.
- Real Lucy resolution and lock verification.
- Checkpoint filesystem snapshot and restore.
- Artifact approval UI and sandboxed review execution.
- Cross-record transactions, cross-process audit locking, and migrations.
- Full Web UI and WebSocket event streaming.
- Real world copying, merging, regeneration, and other world operations.
- Pre-operation checkpoint and crash-snapshot hooks.
- Minecraft 1.12 and latest-version environment implementations.

URL mixin source compilation and automatic fork-world merging are non-goals.

## Development

```bash
gofmt -w cmd internal
go test ./...
go run ./cmd/stratum --data-dir .stratum/data projects create --id demo --name "Demo Project"
go run ./cmd/stratum --data-dir .stratum/data projects list
```
