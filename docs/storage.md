# Metadata Storage

StratumMC's current durable repository stores control-plane metadata beneath a
configurable data directory. The CLI defaults to `.stratum/data`; deployments
should provide an explicit location with `--data-dir`.

Operation metadata is retained as one atomic JSON document per operation in
`operations/`, using the same temporary-file-and-rename path as other entities.

## Layout

```text
<data-dir>/
  projects/<project-id>.json
  rooms/<room-id>.json
  sessions/<session-id>.json
  runtime-observations/<observation-id>.json
  checkpoints/<checkpoint-id>.json
  artifacts/<artifact-id>.json
  artifact-staging-plans/<plan-id>.json
  artifact-apply-plans/<plan-id>.json
  environments/<environment-id>.json
  resource-policies/<policy-id>.json
  audit/events.jsonl
```

Paths are a repository concern. Domain objects contain IDs and explicit
references, never deployment-specific metadata paths. IDs are restricted to
ASCII letters, numbers, `.`, `_`, and `-` so an ID cannot escape its entity
directory.

## JSON records

Projects, rooms, sessions, checkpoints, artifacts, environments, and resource
policies use one JSON document per object. Artifact staging plans are metadata
records for approved-or-rejected staging intent; they do not store payloads.
Artifact apply plans are metadata records describing future placement of
materialized artifacts into runtime-specific target locations. They validate
readiness and target path safety, but do not copy files, mount artifacts, or
execute anything. Runtime observations are append-only diagnostic records with
create/get/list behavior and optional list-by-session filtering. Create
operations reject an existing ID. Updates require an existing object, and
deletes/get operations return typed not-found errors.

Artifact records may include `reviewedBy`, `reviewedAt`, and `reviewReason` when
an explicit metadata-only approval or rejection has been recorded. These fields
are review metadata only; they are not evidence that payload storage exists or
that a file has been copied, mounted, installed, or executed.

## Metadata-only Artifact Creation

`artifacts create` writes project-scoped Artifact metadata with status
`pending` and payload status `metadata-only`. It records the actor as uploader
for attribution, but accepts no file path and creates no hash or payload size.
The command does not upload, hash, copy, mount, install, or execute anything.

Approval remains a separate explicit review step. A metadata-only artifact may
be referenced by a staging plan after approval, but an empty artifact hash in
that plan means no payload blob is linked. Payload upload/import remains future
work.

`artifacts inspect --id <artifact-id>` reads and prints the stored Artifact
metadata without changing it or writing an audit event. It does not read blob
content because payload import and Artifact-to-blob linking remain future work.

## Artifact Blob Storage

Artifact blobs use a separate, explicitly configured storage root rather than
the metadata `--data-dir`. `ArtifactBlobStore` stores immutable content by its
recomputed SHA-256 digest:

```text
<artifact-root>/
  blobs/
    sha256/
      ab/
        <full-sha256>
```

The first two hexadecimal characters shard the blob directory. Repeated writes
of identical content are idempotent, and verification recomputes the stored
digest. Artifact metadata may later reference the returned internal blob
reference. This storage contract does not implement web upload, mounting,
copying, installation, execution, Lucy, MCDR, or Minecraft behavior.

## Artifact Payload Import

`artifacts import-file --id <artifact-id> --file <path> --actor <actor>` imports
one trusted operator-provided local regular file into the separately configured
content-addressed blob store. The BlobStore recomputes SHA-256 and size, then
the pending Artifact metadata records the algorithm, hash, size, internal blob
reference, importing actor, and import time.

The CLI defaults the separate blob root to `.stratum/artifacts`; operators may
set `--artifact-blob-root <path>` before the command.

Only pending Artifacts may import payloads. Repeating the same import is
idempotent; importing different content requires a future explicit replace
operation. Import does not approve, mount, install, copy to a runtime, or
execute the Artifact. Approval remains a separate step, and runtime
staging/copying remains future work. The current payload status value for a
successfully linked blob is `available`.

## Blob Verification

`artifacts blobs verify --sha256 <hash>` opens the configured Artifact BlobStore
and recomputes the stored content hash. It reports the SHA-256 algorithm, hash,
size, internal reference, and `valid` status for intact content; missing and
corrupted blobs return explicit failure statuses. The command is read-only and
does not create the blob root when it is absent.

Verification does not change Artifact metadata, write audit or Operation
records, approve Artifacts, mount or copy blobs, install mods, or execute files.
Runtime staging remains future work.

## Approval Requires Verified Payload

All current Artifact types require an imported payload before approval.
`artifacts approve` checks the linked payload metadata and uses the configured
BlobStore to recompute the SHA-256 digest. Missing metadata, an unsupported
algorithm, an invalid hash, a missing blob, or corrupted content prevents
approval without changing status or review fields.

Approval remains metadata-only after verification. It does not copy, mount,
install, inspect jar contents, or execute the payload; runtime staging remains a
later step.

## Staging Requires Verified Payload

Creating a planned staging record verifies the Artifact's linked SHA-256 blob
again rather than relying on the earlier approval check. Missing, corrupted, or
unverifiable payloads produce rejected staging plans with no Artifact mutation.
These plans are metadata only and do not copy, mount, install, or execute the
blob. Copying into Agent-owned staging requires a separate materialization
request; placement into Minecraft- or MCDR-specific locations remains future
work.

Materialized payloads live separately in the Agent runtime root under
`sessions/<session-id>/artifacts/`. The Agent records them in
`staged-artifacts.json`; no Controller materialization repository is introduced
in this phase. Controller storage retains the staging plan and successful
`artifact.materialized` audit event.

The Agent exposes this manifest through a read-only Session inspection endpoint.
Inspection derives the manifest location from the safe runtime layout and never
accepts an arbitrary filesystem path. Missing manifests represent an empty
materialized-artifact list.

Individual entries may be selected by their staging plan ID through the Agent
API. The lookup reads the same fixed manifest path and introduces no additional
Controller or Agent persistence.

Materialized-file verification uses the target name to derive the expected path
under the Session `artifacts/` directory and requires the manifest's stored path
to match it. The Agent hashes the file read-only and does not update either the
manifest or payload when content is missing or corrupted.

Lists read every `.json` record and return records ordered by filename. A
malformed or unknown-field record stops the list with an actionable repository
error rather than silently dropping metadata.

## Atomic writes

JSON metadata is encoded to a temporary file in the destination directory. The
repository syncs and closes the temporary file before renaming it over the final
path. If encoding or syncing fails, the temporary file is removed and an
existing destination remains unchanged.

This protects individual records from partial writes. Cross-record operations
are not transactional yet; callers must not assume that creating metadata and
an audit event is one atomic unit.

## Audit log

Audit events are append-only JSON Lines in `audit/events.jsonl`. Each event is
encoded on one line, flushed with `fsync`, and read in append order. The current
mutex coordinates writers sharing one repository process. Cross-process file
locking and log rotation remain TODO before multi-controller deployment.

Session lifecycle events use actions such as `session.start` and
`session.freeze`. Their metadata contains `previousState`, `nextState`, and
`result` (`success` or `failure`). Failed operations also include `reason`;
successful crash marking records the supplied crash reason in the same field.

Session state is written atomically before a success audit is appended. These
two files are not a transaction: an audit append failure can be reported after
the state file was updated. A future transactional event/outbox design should
close that gap before multi-controller operation.

Session JSON may also contain `assignedAgentId`, `lastAgentStatus`,
`lastRuntimeMessage`, and `runtimeEndpoint`. These are control-plane
observations and routing metadata; they are not proof that a real process
exists. Agent-backed lifecycle audit events add `agentId`, `agentResult`, and
`agentMessage`.

## Scope

The metadata repository stores metadata only. Artifact blobs use the separate
content-addressed store described above, and `import-file` links Artifact
metadata to those blobs. Live session files, base worlds, checkpoint world
snapshots, secrets, and MCDR/Lucy runtime state remain behind separate storage
and runtime interfaces.

Agent runtime files live under the Agent `--runtime-root`, not the Controller
metadata `--data-dir`. The current runtime layout creates per-session `work`,
`logs`, `config`, `artifacts`, `checkpoints`, and `tmp` directories for future
runtime integrations, but checkpoint backup and cleanup policy remain separate
future work.

Runtime staging manifests under `artifacts/` and `config/` are Agent-side runtime
preparation files. They are not Controller artifact metadata, approval records,
Lucy locks, or durable checkpoint metadata.

Artifact staging plans are Controller metadata under `artifact-staging-plans/`.
They validate approved staging intent only and do not imply that any artifact
payload has been copied, mounted, installed, or executed.

Batch materialized-artifact verification uses safe paths derived from each
Agent manifest entry. It never follows a manifest path to arbitrary storage,
and it leaves both the manifest and staged payloads unchanged.

Materialization readiness correlates Controller records under
`artifact-staging-plans/` with Agent manifest entries by staging plan ID. It
reads current Artifact and Blob metadata but writes neither metadata nor Agent
runtime storage.
