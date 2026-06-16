# Checkpoints

Checkpoints are semantic experiment snapshots that record project state at a specific point in time.

## Current Implementation

The current implementation is **metadata-only**. Checkpoints record:

* Project, Room, and Session identity
* Environment ID and RuntimeProfile ID
* Creator and creation time
* Kind (manual, pre-operation, milestone)
* Status (metadata_only, complete)
* Optional notes
* Optional compact Agent runtime-status snapshot

Checkpoints do **not** currently:

* Copy world files
* Snapshot artifact payloads
* Capture runtime directories
* Enable restore or rollback
* Stop, start, or restart sessions

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

Creation does not:

* Create world backup payloads
* Modify Session state
* Copy runtime-status manifests, paths, or logs
* Stop or pause the runtime
* Repair artifacts or runtime state

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

## Future Phases

Future checkpoint phases may add:

* World state backup and restore
* Artifact snapshot references
* Lucy lock hash capture
* RuntimeProfile validation
* Pre-operation automatic checkpoint creation
* Checkpoint promotion to project milestones
* Rollback workflows

These features are explicitly deferred. Future world checkpoint phases may use
the optional runtime-status snapshot for validation and restore planning. The
current metadata-only implementation does not affect Session lifecycle or
runtime directories.
