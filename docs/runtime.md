# Runtime Profiles

RuntimeProfile is the trusted Agent-side description of a managed runtime. It
defines what child process may be launched, where it runs, how terminal I/O is
captured, how graceful stop is requested, and when force termination is
allowed. RuntimeProfiles are local deployment configuration, not user-provided
commands and not authoritative Controller metadata. The Go model and registry
live in `internal/agent/runtimeprofile`.

The ownership chain is:

```text
Controller
  -> Agent HTTP API
    -> Terminal / Process Runtime Supervisor
      -> trusted child process
```

The Controller owns metadata, operations, resource decisions, permissions,
audit, and authoritative Session state. The Agent owns process handles,
terminal stdin/stdout/stderr, logs, PID and exit status, crash detection,
stop/restart enforcement, resource observation, and future sandboxing.

## Profile types

- `dummy-process`: current enabled Go-native development profile. It creates no OS
  process and produces deterministic in-memory lifecycle logs.
- `terminal`: implemented trusted argv-based local process with managed terminal
  I/O. No production terminal profile is enabled by default.
- `mcdr-managed`: future terminal profile that launches MCDR as the child
  runtime. MCDR may then manage Minecraft internally.
- `minecraft-direct`: future trusted profile that launches a configured
  Minecraft server process without MCDR.

## Safety rules

- No arbitrary shell commands are accepted by default.
- Executables and arguments use an argv array, never an interpolated shell
  string.
- Profiles come only from trusted local configuration or approved built-ins.
- Users cannot supply executable paths, arguments, working directories, or stop
  commands directly.
- Working directories must resolve inside the assigned session runtime root.
- The Agent owns the outer process lifecycle for every profile.
- The Controller owns authoritative metadata state and never hands repositories
  to the Agent.
- MCDR and Lucy cannot directly mutate Controller repositories.

## Current registry behavior

The built-in registry exposes only `dummy-process`, enabled by default. Session
start and restart requests may select a profile by ID; omission resolves to
`dummy-process`. Unknown and disabled IDs fail before runtime start.

The Agent may load additional profiles from a trusted local JSON file with
`--runtime-profiles PATH`. Loading is strict: unknown fields, malformed
durations, invalid profiles, duplicate IDs, and conflicts with built-ins stop
Agent startup. The whole file is registered atomically. Disabled profiles are
retained locally but cannot be listed or selected.

```json
{
  "runtime_profiles": [
    {
      "id": "local-terminal",
      "name": "Trusted local terminal",
      "runtime_type": "terminal",
      "command_argv": ["server", "--nogui"],
      "working_dir": "sessions/example/work",
      "env": {},
      "stop_strategy": "stdin",
      "stop_stdin_command": "stop",
      "graceful_stop_timeout": "30s",
      "force_kill_timeout": "10s",
      "log_mode": "combined",
      "enabled": false,
      "notes": "Enable only after deployment review"
    }
  ]
}
```

This file is machine-owner configuration and may contain executable details.
It must not be generated from user input or exposed through the Controller.
Ordinary lifecycle CLI commands can select only by profile ID; they cannot
provide argv, environment variables, working directories, or stop commands.

Profile discovery returns sanitized metadata only. Executable argv, working
directory, environment entries, and stdin stop commands are not exposed through
the Agent HTTP listing endpoint.

For local development:

```powershell
go run ./cmd/stratum-agent serve --runtime-root .stratum/runtime --runtime-profiles .stratum/runtime-profiles.json
go run ./cmd/stratum --agent-url http://127.0.0.1:8787 agents runtime-profiles --id local
```

## Session Runtime Directory Layout

The Agent owns runtime storage under `--runtime-root`, which defaults to
`.stratum/runtime`. This tree is separate from Controller metadata storage under
`--data-dir`; Controller repositories remain the source of truth for metadata,
while runtime-root contains machine-local runtime files.

Each session start allocates this layout:

```text
runtime-root/
  sessions/
    <session-id>/
      work/
      logs/
      config/
      artifacts/
      checkpoints/
      tmp/
```

Session IDs use the same conservative ASCII path-safety rules as metadata IDs,
and generated paths must remain under runtime-root. The dummy process profile
creates the layout but still starts no OS process. Future MCDR and Minecraft
profiles will use this layout for session-scoped files. Checkpoint backup,
artifact mounting, cleanup policy, and sandboxing remain future work.

## Environment Materialization During Start

Session start prepares Environment runtime directories before starting the
runtime. This materialization step occurs after Environment/RuntimeProfile
compatibility validation and before Agent runtime launch.

Environment materialization:

- Creates session-specific directories (config/, world/, logs/, mods/) under
  runtime-root/sessions/<session-id>/.
- Records informational metadata about the Environment (Minecraft version, Java
  version, loader type, loader version, server core, MCDR/Carpet requirements,
  RuntimeProfile).
- Returns preparation status to the Controller for Operation metadata recording.

Environment materialization does not:

- Install Java, Minecraft, Fabric, or Carpet executables.
- Download mod or datapack files.
- Call Lucy for dependency resolution.
- Start MCDR or Minecraft processes.
- Execute any user-provided code.
- Modify Session state directly (the Controller owns state transitions).

If materialization fails, session start fails before Agent runtime launch. The
Session state remains unchanged (e.g., created or stopped). Operation metadata
records materialization status and error details if available.

Repeated start or restart operations tolerate already-prepared directories.
Directory creation is idempotent. The manual `environments materialize` CLI
command remains available for explicit preparation outside lifecycle workflows.

## Runtime Artifact and Config Staging

Runtime staging is Agent-owned preparation inside a Session runtime layout. The
current helper computes safe paths under `artifacts/` and `config/` and may write
small manifest stubs at `artifacts/staged-artifacts.json` and
`config/staged-config.json`. Staged names must be relative, must not traverse
outside the session layout, and must use conservative path characters.

Staging is not artifact approval, Lucy package installation, mod mounting, or
config preset application. The helper does not copy real artifact payloads from
Controller storage, expose arbitrary file writes to users, or execute staged
files. Future Artifact Manager and Lucy integration may populate these
directories after approval and sandboxing rules are defined.

## Approved Artifact Staging Contract

Artifact staging plans are Controller metadata records that validate whether an
approved Artifact may be staged into a Session. They record the target staging
name, staging kind, artifact status and hash, actor, and whether the plan was
accepted or rejected. Creating a plan does not copy payloads, mount mods, call
Lucy, call the Agent, or execute files.

Only approved artifacts with a currently verified payload are planned. Pending,
rejected, and deprecated artifacts produce rejected metadata plans with audit
history.

## Staging Requires Verified Payload

Staging plan creation recomputes and verifies the linked blob through the
content-addressed BlobStore. An approved Artifact with missing metadata, an
unsupported algorithm, an invalid hash, a missing blob, corrupted content, or
metadata that does not match the blob produces a rejected plan.

The plan remains Controller metadata only. Plan creation does not copy, mount,
install, inspect, or execute payloads. Copying into Agent-owned staging requires
the separate explicit materialization action below.

## Artifact Materialization

`artifacts staging materialize` explicitly sends a reverified planned payload
to the Agent. The Agent independently checks its SHA-256 hash and size, rejects
unsafe or symlinked targets, and atomically copies it under
`sessions/<session-id>/artifacts/<target-name>`. Existing identical content is
an idempotent success; different content is never overwritten.

Materialization updates the Agent-owned `staged-artifacts.json` manifest. It is
not installation, mounting, loading, or execution: Minecraft and MCDR do not
see the file, and Lucy is not involved. Moving materialized files into
runtime-specific locations remains future work.

## Inspecting Materialized Artifacts

`sessions artifacts --id <session-id> --agent-url <url>` reads the Agent-owned
artifact manifest and reports the materialized entries for that Session. A
missing manifest returns an empty result. Inspection is read-only: it does not
verify payload files, install, mount, load, execute, or inspect jar contents.

## Inspecting One Materialized Artifact

`sessions artifacts inspect --id <session-id> --plan <staging-plan-id>` performs
a read-only lookup by Session and staging plan ID. It reads the same Agent-owned
manifest and returns one entry; a missing manifest or plan returns not found.
The command does not verify, install, mount, load, or execute the artifact.

## Verifying Materialized Artifacts

`sessions artifacts verify --id <session-id> --plan <staging-plan-id>` performs
a read-only integrity check. The Agent resolves the file from the safe Session
artifact layout, recomputes its SHA-256 hash and size, and compares both with
the Agent-owned manifest entry. Results are `valid`, `missing`, or `corrupted`.

Verification is intended to detect runtime staging corruption or manual
tampering. It does not repair, install, mount, load, inspect, or execute the
artifact.

### Batch Verification of Materialized Artifacts

`sessions artifacts verify-all --id <session-id>` asks the Agent to recompute
SHA-256 hashes for every entry in the session materialization manifest. The
read-only result summarizes valid, missing, corrupted, and malformed entries;
the CLI exits non-zero when any entry is not valid. This operator health check
does not install, mount, load, repair, or execute artifacts.

### Materialization Readiness

`artifacts staging readiness --session <session-id>` is a read-only diagnostic
before future apply or mount work. It combines current Controller staging and
Artifact/Blob metadata with the Agent batch verification result, then reports
missing, corrupted, stale, or unknown materialized entries. It does not install,
mount, load, repair, or execute artifacts and does not call Lucy, MCDR, or
Minecraft.

## Artifact Approval

Artifact approval is metadata-only review. `artifacts approve` and `artifacts
reject` transition pending artifact metadata, record reviewer and reason fields,
and append audit events. Approval first requires complete imported payload
metadata and successful SHA-256 verification through the content-addressed
BlobStore. Missing or corrupted payloads remain pending. Approval does not copy,
mount, install, or execute payloads; it only makes the Artifact eligible for
future staging plans. Rejected artifacts cannot be staged. `artifacts
import-file` links a trusted local file to pending Artifact metadata without
copying it into an Agent runtime; runtime staging and mounting remain future
work.

`artifacts blobs verify` only recomputes a content-addressed blob's SHA-256. It
does not mutate Artifact metadata, approve content, or interact with Agent
runtime directories.

`artifacts create` creates the pending metadata record used by this workflow.
It records project ownership, type, creator, and an explicit `metadata-only`
payload status. It accepts no path and does not produce a placeholder hash.

## Managed terminal executor

The Agent uses Go `os/exec` directly with `command_argv`; it never invokes a
shell. Terminal profiles without `working_dir` use the session `work/`
directory. Profiles with `working_dir` must use a relative path beneath the
configured Agent runtime root. Absolute paths, traversal outside that root,
missing directories, and non-directory targets are rejected.

The child environment is intentionally small. The Agent inherits only basic
host path and temporary-directory variables needed for cross-platform startup
(`SystemRoot`, `WINDIR`, `TEMP`, `TMP`, `HOME`, and `USERPROFILE` when present),
then adds trusted profile `env` entries. Session requests cannot add variables.

Stdout and stderr are captured with source prefixes into a bounded in-memory
log buffer. Old entries are discarded at the retained byte limit, and the logs
endpoint supports a tail `maxBytes` limit.

Stop behavior follows the trusted profile:

- `stdin` writes `stop_stdin_command` plus a newline and waits for the graceful
  timeout.
- `terminate` requests an OS interrupt where supported and waits for the
  graceful timeout.
- `none` sends no graceful command.
- Any strategy that does not exit in time is force-killed and bounded by
  `force_kill_timeout`.

Unexpected zero-code exit is reported as `exited`; unexpected non-zero exit is
reported as `crashed` with exit code and error. The Agent does not mutate the
Controller Session record. Runtime reconciliation remains future work.

## Runtime Observation and Reconciliation Contract

`RuntimeObservation` is a computed comparison between authoritative Controller
Session metadata and an Agent runtime status. It detects cases such as a
Controller-running Session whose process exited or crashed, a stopped Session
whose runtime is still running, unknown Agent state, assigned-Agent mismatch,
and runtime-profile mismatch when an expected profile is available.

The result contains a mismatch type, severity, and recommended action. These
are diagnostic values only: observation never mutates Session state and never
stops, restarts, or marks a runtime crashed. Operators can inspect the current
comparison with:

```powershell
go run ./cmd/stratum --data-dir .stratum/data --agent-url http://127.0.0.1:8787 sessions observe --id demo-session
```

The explicit `sessions reconcile mark-stopped` operation may repair Controller
metadata after human confirmation. It requires an actor and reason, creates an
Operation and audit trail, and may attach the current observation when an Agent
URL is provided. It never stops or kills the Agent runtime; process control is
a separate lifecycle action. Other reconciliation actions and automatic repair
remain future work.

## Persisted Runtime Observations

`sessions observe --id SESSION` computes and persists a RuntimeObservation by
default. The record captures what the Controller believed about Session metadata
and what the Agent reported about the runtime at one point in time, including
mismatch type, severity, recommended action, runtime profile, process details,
and diagnostic metadata.

Persisted observations are diagnostic history only. They do not mutate
authoritative Session state, do not stop or restart Agent runtimes, do not create
Operation records, and do not perform automatic repair. They may inform later
manual reconciliation. Automatic reconciliation remains future work.

Operators can inspect persisted history with:

```powershell
go run ./cmd/stratum --data-dir .stratum/data runtime-observations list
go run ./cmd/stratum --data-dir .stratum/data runtime-observations list --session demo-session
go run ./cmd/stratum --data-dir .stratum/data runtime-observations inspect --id runtime-observation-demo-session-123
```

`sessions reconcile stop-runtime` performs that separate Agent action. It
requires an Agent URL, actor, and reason; inspects the runtime first; and records
the observation and Agent result in the Operation and audit trail. It does not
change Controller Session state or invoke `mark-stopped` automatically.
Already stopped known runtimes follow the Agent's existing idempotent stop
behavior; unknown runtimes fail clearly. No checkpoint is created, and MCDR
does not become the lifecycle owner.

## Manual Mark-Crashed Reconciliation

`sessions reconcile mark-crashed` is an explicit human-confirmed Controller
metadata repair. It may use and persist an Agent observation when an Agent URL is
provided, but it does not stop, kill, restart, or otherwise mutate the Agent
runtime. It also does not create checkpoint backups.

Operators may combine `sessions observe`, `sessions reconcile mark-crashed`,
`sessions reconcile stop-runtime`, and `sessions reconcile mark-stopped` as
separate manual steps when Controller metadata and Agent runtime state diverge.
Automatic reconciliation remains future work.

## Artifact Apply Dry-Run

Agent artifact apply dry-run is a read-only diagnostic that computes would-be
target placement actions without copying, mounting, installing, or executing
artifacts. It validates that the materialized artifact file exists, matches the
expected hash and size, and that the target path is safe (no traversal, not
absolute).

The dry-run returns:

- status: `ready`, `not_ready`, or `error`,
- action: currently `would_copy`,
- source runtime relative path,
- planned target runtime relative path,
- issues list.

Dry-run does not create target directories, copy files, modify manifests,
inspect jar contents, or execute artifacts. Dry-run is invoked through the Agent
API at `POST /v1/artifacts/apply/dry-run` or via CLI:

```powershell
go run ./cmd/stratum --data-dir .stratum/data --agent-url http://127.0.0.1:8787 artifacts apply dry-run --plan <apply-plan-id> --actor bryan
```

## Artifact Apply Execution

Agent artifact apply execution copies a verified materialized artifact to the
computed runtime target path. Apply execution:

1. Runs dry-run validation first,
2. Fails if dry-run is not ready,
3. Computes source path from materialized artifact manifest,
4. Computes target path under the session runtime layout,
5. Creates parent target directory if needed,
6. Copies file bytes from source to target,
7. Recomputes hash of target file and ensures it matches expected payload hash,
8. Returns apply result.

Supported target roots and their mappings:

- `mods` -> `work/mods/`
- `config` -> `config/`
- `datapacks` -> `work/datapacks/`
- `plugins` -> `work/plugins/`
- `schematics` -> `work/schematics/`
- `worlds` -> `work/worlds/`
- `custom` -> `work/custom/`

The apply result includes:

- status: `applied` or `failed`,
- action: currently `copy`,
- source absolute path,
- target absolute path,
- copied bytes,
- verified target hash,
- issues list.

Apply execution does not install, load, or execute artifacts in a running
Minecraft server. It only copies files inside the Agent-owned session runtime
layout. Apply execution is invoked through the Agent API at
`POST /v1/artifacts/apply/execute` or via CLI:

```powershell
go run ./cmd/stratum --data-dir .stratum/data --agent-url http://127.0.0.1:8787 artifacts apply execute --plan <apply-plan-id> --actor bryan
```

After successful apply, the Agent writes an applied artifact record to
`artifacts/applied-artifacts.json` inside the session runtime layout. Each
record includes apply plan ID, session ID, artifact ID, staging plan ID, source
and target runtime relative paths, target root, target relative path, payload
algorithm, payload hash, payload size, action (copied or already_present),
status, actor ID (if available), and applied timestamp.

Checkpoint creation and rollback remain future work.

### Inspecting Applied Artifacts

Applied artifact inspection is read-only. It reports which files were copied
into runtime target paths. Inspection does not load, install, execute, or
activate artifacts:

```powershell
go run ./cmd/stratum --data-dir .stratum/data --agent-url http://127.0.0.1:8787 sessions applied-artifacts --id demo-session

go run ./cmd/stratum --data-dir .stratum/data --agent-url http://127.0.0.1:8787 sessions applied-artifacts inspect --id demo-session --plan <apply-plan-id>
```

The list command returns all applied artifact records for a session. Missing
manifest returns empty list. The inspect command returns one record by apply
plan ID. Missing record returns not-found error. Malformed manifest returns
structured error.

Inspection does not verify file integrity, repair target files, delete applied
artifacts, create operations, or create audit events beyond existing read-only
diagnostics. Cleanup, rollback, checkpointing, Lucy integration, MCDR
integration, and Minecraft runtime hot-reload remain future work.

### Verifying Applied Artifacts

Applied artifact verification is read-only. It recomputes the SHA-256 hash of
the applied target file and checks target file integrity against the
Agent-owned applied artifact manifest:

```powershell
go run ./cmd/stratum --data-dir .stratum/data --agent-url http://127.0.0.1:8787 sessions applied-artifacts verify --id demo-session --plan <apply-plan-id>
```

Verification is intended to detect runtime target corruption or manual
tampering after apply execution. Missing manifest returns not-found. Missing
apply plan entry returns not-found. Missing target file returns status=missing.
Hash mismatch returns status=corrupted. Hash match returns status=valid. The
CLI exits non-zero when status is not valid.

Verification does not install, load, execute, repair, or hot-reload artifacts.

### Batch Verification of Applied Artifacts

Batch verification is read-only. It recomputes SHA-256 hash for every applied
target file in the session manifest and checks target file integrity:

```powershell
go run ./cmd/stratum --data-dir .stratum/data --agent-url http://127.0.0.1:8787 sessions applied-artifacts verify-all --id demo-session
```

Batch verification is intended for operator health checks before runtime start
or future checkpoint/apply workflows. Missing manifest returns empty summary.
The CLI exits non-zero when any entry is not valid. Batch verification does not
install, load, execute, repair, or hot-reload artifacts.

### Pre-start Artifact Readiness Gate

When `sessions start` uses `--agent-url`, StratumMC performs a read-only artifact
gate before Agent prepare/start calls or Session state transitions. Sessions
with no staging plans and no applied artifact manifest are allowed. Otherwise,
staging readiness must be `ready` and every applied artifact must verify as
valid; missing, corrupted, or unverifiable files block startup.

The gate does not materialize, apply, install, load, repair, remove, or execute
artifacts. It does not create checkpoints or call Lucy, MCDR, or Minecraft. It
prepares the lifecycle path for future real runtime startup.

## MCDR RuntimeProfile example

An example disabled MCDR-managed profile exists in
`docs/runtime-profiles/mcdr-managed.example.json`:

```json
{
  "id": "mcdr-managed-example",
  "name": "MCDR Managed Example",
  "runtimeType": "terminal",
  "commandArgv": ["python", "-m", "mcdreforged"],
  "workingDir": "",
  "stopStrategy": "stdin",
  "stopStdinCommand": "!!MCDR stop",
  "gracefulStopTimeout": "30s",
  "forceKillTimeout": "10s",
  "logMode": "combined",
  "enabled": false,
  "notes": "Example only. Agent would supervise MCDR as a child runtime; MCDR is not the lifecycle owner."
}
```

This profile is disabled and example-only. It demonstrates how MCDR would fit as
an Agent-supervised child runtime in the future:

- Stratum Agent launches MCDR using `python -m mcdreforged` as a terminal child
  process.
- Agent owns stdout/stderr/stdin, PID, exit code, crash detection, and process
  supervision.
- MCDR may manage Minecraft internally and expose plugins/in-game commands.
- MCDR does not own the Stratum Session lifecycle.
- Controller does not call MCDR directly for start/stop operations.
- Agent supervises graceful stop via stdin (`!!MCDR stop`) and enforces
  timeouts.

Real MCDR integration, Minecraft launch, Python environment setup, MCDR plugin
development, and Lucy integration remain future work. Do not enable this profile
without MCDR installation and proper environment configuration.

## MCDR Runtime Directory Layout

StratumMC Agent manages an MCDR-specific directory structure under the session
work directory for future MCDR-managed RuntimeProfiles. Directory preparation
does not start MCDR, invoke Python, or generate config files. The layout is
Agent-owned and follows session runtime safety rules.

Suggested layout:

```text
runtime-root/
  sessions/
    <session-id>/
      work/
        mcdr/
          config/
          plugins/
          server/
          logs/
          tmp/
```

The Agent provides helpers to compute MCDR paths from `SessionRuntimeLayout` and
create these directories idempotently. All paths remain under the session
runtime root. Path traversal is rejected through existing session ID validation.

MCDR directory preparation:
- Does not start MCDR or Minecraft.
- Does not invoke Python or install dependencies.
- Does not generate server.properties, MCDR config, or plugin manifests.
- Does not call Lucy for dependency resolution.
- Is idempotent and safe to repeat.

Future work may populate `config/`, install plugins to `plugins/`, place
Minecraft server jar in `server/`, and launch MCDR through a trusted enabled
RuntimeProfile. MCDR remains a future Agent-supervised child runtime under
Stratum process lifecycle control.

### MCDR Layout Manifest

After directory creation, the Agent can optionally write
`work/mcdr/mcdr-layout.json` to record prepared directory paths:

```json
{
  "session_id": "session-1",
  "created_at": "2026-06-14T12:00:00Z",
  "mcdr_root": "/runtime/sessions/session-1/work/mcdr",
  "config_dir": "/runtime/sessions/session-1/work/mcdr/config",
  "plugins_dir": "/runtime/sessions/session-1/work/mcdr/plugins",
  "server_dir": "/runtime/sessions/session-1/work/mcdr/server",
  "logs_dir": "/runtime/sessions/session-1/work/mcdr/logs",
  "tmp_dir": "/runtime/sessions/session-1/work/mcdr/tmp",
  "status": "prepared",
  "notes": "MCDR runtime directories are prepared only; MCDR and Minecraft are not started."
}
```

This manifest is informational only:
- It records prepared directories for diagnostics and runtime checks.
- It is not MCDR config (`config.yml`).
- It is not Minecraft config (`server.properties`).
- It does not imply MCDR has been installed or started.
- Future work may use it for preparation verification or recovery workflows.

Manifest generation is idempotent and safe to repeat.

## Environment Metadata

Environment is a Controller-owned metadata concept that describes the intended
Minecraft and runtime dependency context for a Session. An Environment record
includes:

- Minecraft version (e.g., `1.17.1`, `1.12.2`)
- Java version (e.g., `16`, `8`)
- Loader type (`none`, `fabric`, `liteloader`, `forge`, `quilt`, `custom`)
- Server core (`vanilla`, `fabric`, `carpet`, `paper`, `custom`)
- MCDR and Carpet requirement flags
- Optional Lucy manifest/lock references
- Optional RuntimeProfile ID association
- Descriptive metadata

Example:

```json
{
  "id": "env-1-17-fabric",
  "name": "1.17 Fabric Carpet",
  "minecraftVersion": "1.17.1",
  "javaVersion": "16",
  "loaderType": "fabric",
  "serverCore": "carpet",
  "mcdrRequired": false,
  "carpetRequired": true,
  "createdAt": "2026-06-14T12:00:00Z",
  "updatedAt": "2026-06-14T12:00:00Z"
}
```

Environments are metadata-only in the current implementation:

- Creating an Environment does not install Java, Minecraft, Fabric, or Carpet.
- It does not call Lucy for dependency resolution.
- It does not generate MCDR config or server.properties.
- It does not launch MCDR or Minecraft.
- It does not modify Artifact staging or apply behavior.

RuntimeProfile still controls process startup. Lucy may later resolve and
install Environment dependencies. MCDR/Minecraft launch remains future work.
Sessions may reference Environment IDs to describe intended context, but
Environment metadata does not currently drive automated installation or
session startup behavior.

Lucy remains a non-intrusive dependency, manifest, and lock-management
integration. It is not a process supervisor or session lifecycle controller.

## Environment and RuntimeProfile Linking

Environment may declare a `runtimeProfileId` field to describe the intended
RuntimeProfile for sessions using that Environment. This is optional metadata
only. The link allows:

- Environment metadata to record the expected Agent-side runtime launch profile
- Future validation of Environment/RuntimeProfile compatibility
- Documentation of which runtime profile should be used for an Environment

The field is optional. If provided, it must be a safe ID without path separators.
It does not require the RuntimeProfile to exist or be enabled at Environment
creation time.

Session start behavior is not changed by this field. Future integration may use
Environment `runtimeProfileId` to validate or select the correct RuntimeProfile
when starting a session.

Environment may also declare `runtimeProfileRequired` to indicate that sessions
must use the specified RuntimeProfile. This is not currently enforced.

MCDR, Minecraft, Lucy integration, dependency installation, and automated runtime
selection remain future work.

## Environment RuntimeProfile Compatibility

Session start and restart validate Environment/RuntimeProfile compatibility:

- If Environment declares `runtimeProfileId`, sessions may use it as the default
  when no explicit runtime profile is requested.
- If Environment declares `runtimeProfileRequired: true`, the session must use
  the Environment's `runtimeProfileId`. Explicit mismatches fail before Agent
  start.
- Missing Environment blocks start with a clear error.
- Compatibility validation sets Operation metadata:
  - `environmentId`
  - `environmentRuntimeProfileId`
  - `environmentRuntimeProfileRequired`
  - `requestedRuntimeProfileId`
  - `selectedRuntimeProfileId`
  - `runtimeProfileCompatibilityStatus` (`ok` or `failed`)
  - `runtimeProfileCompatibilityReason` (failure reason if status is `failed`)

This validation is metadata-only. It does not:

- Install Java, Minecraft, Fabric, Carpet, or MCDR
- Call Lucy for dependency resolution
- Launch MCDR or Minecraft
- Validate host availability
- Resolve or materialize artifacts

RuntimeProfile existence and enablement are still validated by Agent registry
behavior. Environment validation happens first, before Agent start is attempted.
