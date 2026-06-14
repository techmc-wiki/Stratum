# MVP Scope

The MVP establishes a small, testable control-plane core without launching a
Minecraft server.

## Completed phases

- Go repository skeleton and core domain models.
- File-backed metadata storage and append-only audit history.
- Session lifecycle service and resource-policy enforcement.
- Successful Session start/restart RuntimeProfile selection persisted for
  inspection and metadata-only checkpoint capture.
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
- Per-session Agent runtime directory allocation under `--runtime-root`.
- Agent-side artifact/config staging path helpers and manifest stubs.
- Metadata-only approved artifact staging plans with audit history.
- Metadata-only artifact approval/rejection with reviewer audit history.
- Metadata-only pending artifact creation with project and actor attribution.
- Read-only Artifact metadata inspection through the CLI.
- Separate content-addressed Artifact blob storage with SHA-256 verification.
- Trusted local-file payload import for pending Artifact metadata.
- Read-only Artifact blob verification through the CLI.
- Verified-payload requirement for Artifact approval.
- Verified-payload requirement for planned Artifact staging records.
- Agent-side verified Artifact materialization with a staging manifest.
- Read-only Agent inspection of materialized Artifact manifests.
- Read-only lookup of one materialized Artifact by staging plan ID.
- Read-only integrity verification of materialized Artifact files.
- Read-only batch verification of a Session's materialized Artifact manifest.
- Read-only materialization readiness across Controller staging metadata and
  Agent artifact verification.
- Metadata-only artifact apply plans with validated target paths, mapped kinds,
  and readiness-checked materialization references. Plans do not copy, mount, or
  execute anything.
- Agent-side artifact apply dry-run that validates materialized file integrity,
  computes would-be target placement paths, and returns readiness status without
  copying, mounting, installing, or executing artifacts.
- Agent-side artifact apply execution that copies verified materialized
  artifacts to computed runtime target paths (mods, config, datapacks, plugins,
  schematics, worlds, custom) without installing, loading, or executing
  artifacts in a running Minecraft server.
- Agent-side read-only inspection of applied artifact records. The Agent writes
  an applied artifact manifest (`artifacts/applied-artifacts.json`) after each
  successful apply execution. Inspection commands list all applied artifacts for
  a session or inspect one record by apply plan ID. Inspection does not verify,
  repair, delete, install, load, or execute artifacts.
- Agent-side read-only verification of applied artifact target integrity.
  Verification recomputes SHA-256 hash of the applied target file and checks
  integrity against the applied artifact manifest. Verification detects runtime
  target corruption or manual tampering. Verification does not repair, install,
  load, or execute artifacts.
- Agent-side read-only batch verification of all applied artifacts in a session.
  Batch verification recomputes SHA-256 hash for every applied target file in
  the session manifest and checks target file integrity. Batch verification is
  intended for operator health checks before runtime start or future
  checkpoint/apply workflows. Batch verification does not install, load,
  execute, repair, or hot-reload artifacts.
- Read-only pre-start artifact readiness gate for remote Session start. It
  combines staging readiness and applied artifact verification, records the
  result in Operation/audit metadata, and blocks unsafe startup before Agent
  prepare/start or Session state mutation.
- Agent-side Environment materialization that prepares runtime directory
  structure based on Environment metadata (Minecraft version, Java version,
  loader type, server core, MCDR/Carpet requirements, RuntimeProfile). Session
  start and restart call Environment materialization after
  Environment/RuntimeProfile compatibility validation and before Agent runtime
  launch. Materialization creates the standard Session runtime layout plus
  world/ and mods/, writes an informational manifest at
  `config/environment-materialization.json`, and records metadata. It does not
  install Java, Minecraft, Fabric, or Carpet, does not download files, does not
  call Lucy, and does not start MCDR or Minecraft. Materialization failure blocks
  session start before Agent runtime launch.
- Agent-side read-only Session start readiness predicate. It summarizes runtime
  directories, Environment manifest status, process state, required MCDR layout,
  and applied artifact verification without repairing, installing, or starting
  anything. Controller start calls the predicate after Environment
  materialization and blocks runtime launch when readiness fails. Restart of a
  running Session now explicitly stops and persists stopped state before
  materialization and readiness, then calls Agent start only when ready.
  Sequence diagnostics are recorded in Operation metadata.
- Environment metadata explicit update with optimistic conflict protection. Update
  requires --expected-updated-at and fails with a conflict error if the current
  updated_at does not match. Update mutates Environment metadata only. It does
  not reinstall, rematerialize, restart, or automatically update Rooms or
  Sessions referencing the Environment.
- Checkpoint metadata-only creation with CLI create/list/inspect commands.
  Checkpoints record Session/Environment/RuntimeProfile identity, creator, kind,
  status, notes, and an optional compact read-only Agent runtime-status snapshot.
  They do not copy world files, snapshot artifacts, backup runtime directories,
  repair runtime state, stop sessions, or support restore/rollback. Checkpoint
  creation writes a checkpoint.created audit event.

The current HTTP Agent maintains safe cross-platform dummy runtimes, captures
lifecycle logs, reports running/stopped process observations, and counts active
runtimes in resource reports. Lifecycle decisions remain in the Controller. No
command starts Minecraft, MCDR, Lucy, or another JVM process.

## Next phase: Additional Explicit Reconciliation

RuntimeProfile loading, runtime mismatch detection, persisted observation
history, explicit mark-stopped, stop-runtime, and mark-crashed reconciliation,
per-session runtime directory allocation, and internal runtime staging helpers
are implemented. Metadata-only artifact staging plans define which approved
artifacts may later be staged, and artifact review can approve or reject pending
metadata. Artifact metadata can be created without inventing a payload hash.
The next phase may add another narrowly scoped Agent runtime
capability. It must not perform automatic repair or accept arbitrary
user-supplied commands.

A later phase may add **MCDR RuntimeProfile v0**, where the Agent launches MCDR
as a trusted child process. An example disabled MCDR-managed profile exists in
`docs/runtime-profiles/mcdr-managed.example.json` to demonstrate the intended
integration. Agent provides MCDR runtime directory layout helpers that compute
and create MCDR-specific directories under session work directory; directory
preparation does not start MCDR or invoke Python. MCDR may manage Minecraft
internally, but it will not replace Agent process supervision or become the
Controller's lifecycle manager.

## Included

- Project, Room, Session, Checkpoint, Artifact, Environment, ResourcePolicy,
  AuditEvent, and Operation domain models.
- Explicit session state transitions and resource-policy decisions.
- SHA-256 artifact hashing and pending-by-default metadata.
- Checkpoint metadata construction plus list/rollback service stubs.
- Checkpoint metadata-only create/list/inspect CLI commands and audit events.
- Environment domain stub with validation, repository, and CLI support.
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
- Example disabled MCDR-managed RuntimeProfile in
  `docs/runtime-profiles/mcdr-managed.example.json`.
- Example-only GTMC 1.12, 1.17, and latest Environment metadata templates in
  `docs/environments/`; they are not automatically seeded or resolved.
- MCDR runtime directory layout helpers in `internal/agent/process`.
- MCDR, storage, and runtime-agent interface stubs.
- Stratum-owned Lucy adapter boundary for capability discovery, planning,
  locking, and status checks, plus a deterministic no-I/O adapter. Real Lucy
  resolution, CLI/package integration, manifests, locks, and downloads remain
  deferred.
- Non-executing MCDR config stub contract with canonical Session-relative paths
  and validation, plus optional atomic serialization to the informational
  `work/mcdr/mcdr-config-stub.json` planning manifest. It does not write real
  MCDR or Minecraft configuration, install dependencies, invoke Python/Lucy,
  or start processes.
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
