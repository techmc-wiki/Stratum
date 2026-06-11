# MVP Scope

The MVP establishes a small, testable control-plane core without launching a
Minecraft server.

## Current phase

The current phase adds durable, resource-aware session lifecycle control. The
controller can persist and audit lifecycle intent, but no command in this phase
starts Minecraft, MCDR, Lucy, or another JVM process.

## Included

- Project, Room, Session, Checkpoint, Artifact, Environment, ResourcePolicy,
  and AuditEvent domain models.
- Explicit session state transitions.
- Resource-policy decisions with actionable denial reasons.
- SHA-256 artifact hashing and pending-by-default metadata.
- Checkpoint metadata construction plus list/rollback service stubs.
- A Minecraft 1.17 Fabric + MCDR + Carpet environment template.
- In-memory repositories and durable file-backed metadata repositories.
- Atomic JSON persistence for projects, rooms, sessions, checkpoints,
  artifacts, environments, and resource policies.
- Append-only JSONL audit persistence and reload.
- Persistent CLI create/list flows rooted at configurable `--data-dir`.
- Resource-aware session lifecycle operations with persistent state changes.
- Success and failure audit events for every lifecycle request.
- Checkpoint list/get CLI commands.
- MCDR, Lucy, storage, and runtime-agent interfaces.
- Minimal CLI commands and HTTP health endpoint.
- Standard-library tests for core behavior.

## Deferred

- Real MCDR integration and process supervision.
- Real Lucy resolution and lock verification.
- Checkpoint filesystem snapshot and restore.
- Artifact approval UI and sandboxed review execution.
- Cross-record transactions, cross-process audit locking, and repository
  migrations.
- Authentication and authorization transport.
- Full Web UI and WebSocket event streaming.
- Real world copying, merging, regeneration, and other world operations.
- Pre-operation checkpoint and crash-snapshot hooks for lifecycle actions.
- Minecraft 1.12 and latest-version environment implementations.

URL mixin source compilation and automatic fork-world merging are non-goals.

## Development

```bash
gofmt -w cmd internal
go test ./...
go run ./cmd/stratum --data-dir .stratum/data projects create --id demo --name "Demo Project"
go run ./cmd/stratum --data-dir .stratum/data projects list
```
