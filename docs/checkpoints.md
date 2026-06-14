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

Checkpoints do **not** currently:

* Copy world files
* Snapshot artifact payloads
* Capture runtime directories
* Enable restore or rollback
* Stop, start, or restart sessions

## Service Layer

Checkpoint metadata orchestration is handled by `internal/service/checkpointsvc`.

The service layer:

* Loads Session to derive Project/Room/Environment identity
* Creates metadata-only Checkpoint via repository
* Writes `checkpoint.created` audit events
* Validates actor presence and checkpoint ID safety

Repository remains storage-only (create/get/list/list-by-session).

## Creation

```bash
stratum checkpoints create --id <checkpoint_id> --session <session_id> --actor <actor> --notes <notes>
```

Creation loads the Session and derives:

* `project_id` from Session.ProjectID
* `room_id` from Session.RoomID
* `environment_id` from Session.EnvironmentID
* `runtime_profile_id` from Session.RuntimeProfileID

Creation writes a `checkpoint.created` audit event with checkpoint metadata.

Creation does not:

* Create world backup payloads
* Modify Session state
* Call the Agent
* Stop or pause the runtime

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

Shows checkpoint metadata including ID, Project, Room, Session, Environment, RuntimeProfile, creator, kind, status, notes, and creation time.

## Future Phases

Future checkpoint phases may add:

* World state backup and restore
* Artifact snapshot references
* Lucy lock hash capture
* RuntimeProfile validation
* Pre-operation automatic checkpoint creation
* Checkpoint promotion to project milestones
* Rollback workflows

These features are explicitly deferred. The current metadata-only implementation is safe and does not affect Session lifecycle or runtime directories.
