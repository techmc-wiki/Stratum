# Checkpoints

Checkpoints are semantic experiment snapshots that record project state at a specific point in time.

See [Workflows](workflows/) for end-to-end checkpoint usage examples.

## Current Implementation

Checkpoints capture:

* Project, Room, and Session identity
* Environment ID and RuntimeProfile ID
* Creator and creation time
* Kind (manual, pre-operation, milestone)
* Status (metadata_only, complete)
* Optional notes
* Optional compact Agent runtime-status snapshot
* **World state snapshot** (world files packed to zip)
* **World Profile snapshot** (server.properties configuration)

Checkpoints support:

* World restore to target session
* World Profile application during restore
* Audit event creation
* Consistency-level metadata

See [World Profile documentation](world-profile.md) for world configuration capture and restore workflows.

See [CLI Reference](cli-reference.md) for complete checkpoint command documentation.

## Service Layer

Checkpoint metadata orchestration is handled by `internal/checkpoint/service`.

The service layer:

* Loads Session to derive Project/Room/Environment/RuntimeProfile identity
* Creates metadata-only Checkpoint via repository
* Writes `checkpoint.created` audit events
* Validates actor presence and checkpoint ID safety
* Captures RuntimeProfileID from Session when available

Repository remains storage-only (create/get/list/list-by-session).

## Creation

```bash
stratum checkpoints create --id <checkpoint_id> --session <session_id> --actor <actor> --notes <notes>
stratum --agent-url http://127.0.0.1:8787 checkpoints create --id <checkpoint_id> --session <session_id> --actor <actor> --notes <notes>
```

Creation loads the Session and derives:

* `project_id` from Session.ProjectID
* `room_id` from Session.RoomID
* `environment_id` from Session.EnvironmentID
* `runtime_profile_id` from Session.RuntimeProfileID (when available)

RuntimeProfileID capture is metadata-only. It records the selected
RuntimeProfile persisted on Session after a successful start or restart. It
does not:

* Snapshot runtime files or directories
* Infer runtime state when no Agent URL is provided
* Restore or launch runtimes
* Validate profile compatibility beyond what Session already stored

Creation writes a `checkpoint.created` audit event with checkpoint metadata.

When `--agent-url` is provided, creation first calls the Agent's read-only
runtime-status endpoint and stores a compact `runtimeStatusSnapshot`. The
snapshot records directory and Environment manifest presence, Environment and
RuntimeProfile identity, MCDR layout presence, artifact counts, process state,
PID, overall diagnostic status, and issue codes. An Agent runtime-status error
fails creation before checkpoint or audit data is written.

### Consistency Levels

| Level | Description | World Snapshot | Commands |
|-------|-------------|---------------|----------|
| `metadata_only` | Metadata and runtime status only | No | None |
| `stopped` | Stop session → snapshot → restart | Yes (zip + SHA-256) | `save-all flush` (implicit, session stopped first) |
| `best_effort` | `save-all flush` + world snapshot | Yes (zip + SHA-256) | `save-all flush` |
| `command_quiesced` | `save-off` → `save-all flush` → snapshot → `save-on` | Yes (zip + SHA-256) | `save-off`, `save-all flush`, `save-on` |

**`best_effort`:**
- Sends `save-all flush` to flush all chunks to disk (best effort, may fail)
- Takes world snapshot regardless of whether the command succeeded
- Suitable for quick checkpoints where save-off/save-on overhead is undesirable
- Does not guarantee full internal consistency (mod state, async tasks)

**`command_quiesced`:**
- Sends `save-off` to pause Minecraft auto-saves
- Sends `save-all flush` to flush all chunks
- Creates world snapshot (zip with SHA-256)
- Sends `save-on` to resume auto-saves (guaranteed even on snapshot failure)
- Requires agent with `send-command` capability

**Future levels:** `plugin_backup`, `mc_bridge_prepared` (see architecture.md)

**`stopped`:**
- Calls `agent.StopSession` to stop the running session
- Creates world snapshot while session is stopped (guarantees consistent filesystem state)
- Calls `agent.StartSession` with the session's stored `RuntimeProfileID` to restart
- Requires agent client and running session
- Fails if stop fails, snapshot fails, or restart fails
- Suitable for critical pre-operation checkpoints where world consistency is paramount

Creation does not:

* Modify Session state (except `stopped` level, which stops and restarts)
* Copy runtime-status manifests, paths, or logs
* (metadata_only) Create world backup payloads

## Listing

```bash
stratum checkpoints list
stratum checkpoints list --session <session_id>
```

Lists all checkpoints or filters by session.

## Inspection

```bash
stratum checkpoints inspect --id <checkpoint_id>
```

Shows checkpoint metadata including ID, Project, Room, Session, Environment,
RuntimeProfile, creator, kind, status, notes, creation time, and whether a
runtime-status snapshot exists. Compact snapshot diagnostics are shown when
present.

## Restore

```bash
stratum checkpoints restore --checkpoint <checkpoint_id> --target-session <session_id> --actor <actor>
stratum checkpoints restore --checkpoint <checkpoint_id> --target-session <session_id> --actor <actor> --world-dir world
stratum checkpoints restore --checkpoint <checkpoint_id> --target-session <session_id> --actor <actor> --apply-world-profile
stratum checkpoints restore --checkpoint <checkpoint_id> --target-session <session_id> --actor <actor> --auto-stop --auto-start
```

Restore extracts the checkpoint's world snapshot zip into the target session's
work directory, creates a new checkpoint record, and writes an audit event.

**CLI flags:**
- `--checkpoint` — source checkpoint ID (required)
- `--target-session` — target session ID (must be stopped; use `--auto-stop` to auto-stop)
- `--world-dir` — target world directory name (default: `world_restored`)
- `--actor` — user performing the restore (required)
- `--notes` — optional notes for the restored checkpoint
- `--apply-world-profile` — apply the checkpoint's world profile to `server.properties`
- `--apply-world-profile-fields` — comma-separated list of fields for partial merge
- `--auto-stop` — stop the target session before restore
- `--auto-start` — start the target session after restore (uses stored RuntimeProfileID)

**Restore lifecycle:**

1. Restore validates the source checkpoint has `WorldStateRef`
2. Target session must be `stopped` (or use `--auto-stop`)
3. `agent.RestoreWorldSnapshot` unzips to `<session-root>/work/<world-dir>`
4. If `--apply-world-profile`, writes `server.properties` from checkpoint
5. Creates a new `metadata_only` checkpoint entry recording the restore
6. Writes `checkpoint.restored` audit event

Restore does not modify the source checkpoint.

## Pre-Operation Checkpoints

Dangerous operations can automatically create a world snapshot before proceeding:

```bash
stratum sessions restart --id <session_id> --actor <actor> --pre-op-checkpoint
```

When `--pre-op-checkpoint` is set on a session restart:
1. Session is stopped (normal restart behavior)
2. Agent creates a world snapshot while the session is stopped
3. A `KindPreOperation` checkpoint is created with the snapshot reference
4. Session proceeds with normal restart flow

If the checkpoint creation fails, the restart continues (best-effort). The failure is recorded in the operation metadata.

**Supported operations:**
- `sessions restart --pre-op-checkpoint`

**Future support:** `sessions start`, `artifacts apply`

## Future Phases

Future checkpoint phases may add:

* Artifact snapshot references
* Checkpoint promotion to project milestones
* Incremental/differential backups
* Cross-agent restore support
* Integration with Lucy lock hash diff on restore
