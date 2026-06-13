# Controller-Agent Protocol

StratumMC separates control-plane decisions from machine-local execution. The
Controller owns projects, rooms, permissions, scheduling, authoritative session
state, operations, checkpoints, artifacts, and audit history. An Agent owns
machine-local process and terminal lifecycle, logs, runtime observations,
resource reporting, and future filesystem, checkpoint-packing, and sandboxing
work on one runtime host.

## Controller-facing protocol

`internal/agent.AgentClient` is the transport-independent boundary used by
controller services. It exposes:

- session prepare, start, stop, restart, freeze, unfreeze, and inspection;
- fake log collection and resource reporting;
- checkpoint create/restore stubs;
- agent identity and endpoint metadata.

Requests contain domain IDs and environment references, never host-local paths.
A future authenticated HTTP, RPC, or message transport can implement the same
interface without moving lifecycle policy into the agent.

The Agent HTTP API is the machine-local execution boundary. The Agent must
expose runtime inspection, logs, and resource observations so the Controller can
coordinate operations and persist authoritative metadata. The Agent reports
facts; it does not write Controller repositories directly.

## Local HTTP transport

`internal/agent/httptransport` provides a standard-library HTTP server and an
`AgentClient` implementation. The transport converts explicit JSON DTOs to the
agent protocol and back. It does not receive a controller repository and cannot
mutate project, room, session, checkpoint, artifact, or audit metadata.

Current endpoints:

```text
GET  /health
GET  /v1/agent
GET  /v1/agent/resources
POST /v1/sessions/{id}/prepare
POST /v1/sessions/{id}/start
POST /v1/sessions/{id}/stop
POST /v1/sessions/{id}/restart
POST /v1/sessions/{id}/freeze
POST /v1/sessions/{id}/unfreeze
GET  /v1/sessions/{id}/inspect
GET  /v1/sessions/{id}/logs
POST /v1/checkpoints/create-stub
POST /v1/checkpoints/restore-stub
```

Run the development agent with:

```bash
go run ./cmd/stratum-agent serve --listen 127.0.0.1:8787
```

Controller CLI commands use the in-process fake by default. Supplying
`--agent-url http://127.0.0.1:8787` switches all lifecycle, inspection, log, and
resource calls to the HTTP client. The client has a configurable timeout and
validates bounded JSON responses.

## Development authentication

The agent accepts an optional shared token through `--token` or
`STRATUM_AGENT_TOKEN`. When configured, every request must contain
`Authorization: Bearer <token>`. The CLI sends it with `--agent-token`.

This is only a local-development placeholder. It has no identity, rotation,
scope, replay protection, TLS policy, or fine-grained authorization.

## Request IDs

The server preserves `X-Request-ID` or generates one when absent. It returns the
ID in the response header and structured error body, and includes it in server
error logs. The HTTP client generates an ID per request unless the caller uses
`httptransport.WithRequestID` to provide one.

## Agent-internal interfaces

The protocol package also defines `RuntimeAgent`, `ProcessSupervisor`,
`LogStreamer`, `FileOperator`, `ResourceReporter`, and `CheckpointWorker`.
These split machine-local responsibilities so managed terminal runtimes,
filesystem isolation, and checkpoint handling can be implemented independently.

## Process supervision stub

`stratum-agent serve` uses `local.ProcessAgent`, backed by the Go-native
`process.Supervisor`. It does not launch an operating-system command. Each
started session receives an Agent-local runtime record containing a symbolic
process ID, session and agent IDs, runtime status, sanitized command name,
timestamps, exit code, last error, in-memory log reference, and runtime mode.

The only supported server runtime mode is `dummy-process`. The agent does not
accept command text, executable paths, or shell fragments from users or the
Controller. Start creates a cancellable Go runtime handle; stop cancels it and
waits for a clean exit; restart performs stop then start. The supervisor can
also mark a runtime crashed for tests and future exit detection integration.

Dummy logs are deterministic lifecycle messages captured in memory. The HTTP
session inspect and logs endpoints expose these observations. Resource reports
count supervised runtimes currently in `running` state.

This runtime state is Agent-local and ephemeral. The Controller remains the
source of truth for Session and Operation metadata, and the Agent never receives
a Controller repository.

The Agent owns a trusted RuntimeProfile registry. `GET
/v1/agent/runtime-profiles` lists enabled profiles, and start/restart requests
carry only a `runtimeProfileId`. The default is `dummy-process`. Profile
validation requires argv arrays for terminal profiles and rejects common shell
executables; the ordinary CLI never accepts command argv or environment input.
The managed terminal executor uses `os/exec` with argv, allocates per-session
directories under `--runtime-root`, constrains working directories to that root,
captures bounded stdout/stderr logs, and tracks PID, exit code, and unexpected
exit. No production terminal profile is enabled by default. Machine owners may
load reviewed profiles at Agent startup with `--runtime-profiles PATH`; the JSON
loader rejects unknown fields and registers the complete validated file
atomically. Disabled profiles are neither listed nor runnable, and profile
discovery removes argv, working directory, environment, and stdin stop command
values. See `runtime.md` for the format and trust boundary.

The Agent runtime layout includes internal staging helpers for artifact and
config preparation. They compute safe paths and manifests; artifact bytes enter
this area only through the explicit materialization contract below. They do not
approve artifacts, install Lucy packages, mount mods, or execute files.

## Artifact Materialization

The Agent accepts an explicit, size-limited artifact materialization request
containing payload bytes and trusted staging metadata. It validates the target
inside the Session `artifacts/` directory, rejects symbolic-link traversal,
recomputes SHA-256 before and after the atomic write, and updates
`artifacts/staged-artifacts.json`.

This endpoint only creates an Agent-owned staged file. It does not install a
mod, copy into `work/` or `mods/`, call Lucy or MCDR, launch Minecraft, or
inspect jar contents. The current JSON payload transfer is intentionally
limited to 64 MiB; a future remote large-file transport can replace it without
changing runtime ownership.

## Inspecting Materialized Artifacts

`GET /v1/sessions/{id}/artifacts` reads only the known
`artifacts/staged-artifacts.json` path derived from the validated Session
runtime layout. It returns an empty list when no manifest exists and rejects
unsafe Session IDs, symbolic-link paths, malformed manifests, and invalid
manifest entries.

This endpoint confirms what has been staged into the Agent runtime layout. It
does not read artifact payload contents or install, mount, load, execute, or
remove files.

## Inspecting One Materialized Artifact

`GET /v1/sessions/{id}/artifacts/{staging-plan-id}` validates both identifiers
and finds one entry in the Agent-owned materialization manifest. Missing
manifests and missing entries return HTTP 404. The lookup is read-only and never
opens the materialized payload.

## Verifying Materialized Artifacts

`GET /v1/sessions/{id}/artifacts/{staging-plan-id}/verify` validates the
manifest entry and safely derives its file below the Session `artifacts/`
directory. It rejects mismatched manifest paths and symbolic links, then reads
the file only to recompute SHA-256 and size. The response reports `valid`,
`missing`, or `corrupted` with expected and actual values.

The endpoint is diagnostic and read-only. It performs no automatic repair,
installation, mounting, loading, or execution.

### Batch Verification of Materialized Artifacts

`GET /v1/sessions/{id}/artifacts/verify` verifies every artifact entry in the
Agent-owned session manifest and returns aggregate valid, missing, corrupted,
and error counts. A malformed entry is reported without preventing practical
verification of later entries; malformed manifest JSON remains a structured
request error. The endpoint is read-only and never installs or executes files.

### Materialization Readiness

The Controller-side readiness diagnostic consumes the Agent's existing batch
verification response and correlates entries by staging plan ID. The Agent
remains a read-only source of runtime file observations for this check; it does
not mutate manifests, repair files, mount artifacts, or invoke Lucy, MCDR, or
Minecraft.

## Why Agent controls MCDR, not the other way around

MCDR is itself a process that requires supervision. A future trusted
RuntimeProfile may launch MCDR as the child runtime, but the Agent must retain
the outer process handle and terminal boundary.

- If MCDR exits or crashes, the Agent must detect its exit code and report it.
- If Minecraft or MCDR hangs, the Agent must still be able to enforce graceful
  and force-stop deadlines.
- Terminal input, stdout/stderr capture, resource usage, crash recovery, and
  future sandboxing belong to the machine-local Agent.
- MCDR may manage Minecraft console/plugin behavior inside the runtime, but it
  cannot become the authoritative Controller or mutate Controller repositories.

The intended chain is `Controller -> Agent HTTP API -> Runtime Supervisor ->
optional MCDR child -> Minecraft`. The Controller does not call MCDR directly
for its primary lifecycle operations.

## Local fake agent

`internal/agent/local.Fake` remains deterministic and in-process for focused
tests and for CLI use without `--agent-url`. It maintains only
temporary in-memory observations, returns fixed logs and resource capacity, and
can be configured to fail individual operations in tests. It does not:

- launch or inspect a real process;
- call MCDR or Lucy;
- access worlds or uploaded artifacts;
- create or restore a real checkpoint;
- provide a security or isolation boundary.

The fixed resource report contains CPU capacity, memory usage, disk usage, and
the number of sessions observed as running by that fake instance. Failures use
a structured `agent.Error` containing agent ID, operation, and message.

The CLI constructs this fake for each invocation. The long-running HTTP agent
uses the process supervision stub instead. Durable controller metadata
records the assigned agent, last reported result, and endpoint placeholder, but
the fake's in-memory runtime observation is intentionally not durable.

Session metadata keeps `lastAgentStatus` (a protocol result such as `success`)
separate from `lastRuntimeMessage` (operation detail such as `running` or
`stopped`).

## Runtime Observation and Reconciliation Contract

The Controller remains the source of truth for Session metadata. A
`RuntimeObservation` compares that metadata with one Agent `InspectSession`
response and records any mismatch, its severity, and a recommended action.
Observations include process/profile identifiers, exit details, optional
resource data, and enough Controller context to diagnose the difference.

This phase is detection and history only. Computing or persisting an observation
does not change Session state, mark a Session crashed, or stop/restart a
runtime. The `sessions observe --id <session>` command uses the existing Agent
inspect API, persists the comparison, and writes a diagnostic audit event.
Explicit reconcile operations may consume the recommendation after authorization
and audit rules are defined.

## Lifecycle ordering

The Session Lifecycle Service validates transitions and resource policy first,
then invokes the optional agent, then commits final session metadata. If the
agent returns an error, the control-plane state is left unchanged and a failure
audit event records the agent details. Success events include `agentId`,
`agentResult`, and `agentMessage`.

State persistence and audit append are still separate writes. Real remote-agent
work will need explicit reconciliation operations, production authentication,
retry policy, and an outbox/event transaction strategy.

The current transport supplies request IDs, a client timeout, and a shared
token placeholder. MCDR, Minecraft, Lucy, runtime reconciliation, production
authentication, TLS configuration, retry policy, durable Agent runtime
recovery, and transactional outbox behavior remain TODO.

## Artifact Apply Dry-Run

Agent dry-run reads apply plans and materialized artifacts to compute would-be
target placement actions without copying, mounting, installing, loading, or
executing artifacts. Dry-run validates:

- session runtime layout exists,
- materialized artifact manifest contains the staging plan,
- materialized artifact file exists and matches expected hash/size,
- target relative path is safe (not absolute, no traversal),
- target root is supported (mods, config, datapacks, plugins, schematics,
  worlds, custom).

The dry-run result includes:

- status: `ready`, `not_ready`, or `error`,
- action: `would_copy` (future: `would_link`),
- source runtime relative path,
- planned target runtime relative path,
- issues.

Dry-run is read-only. It does not create target directories, copy files, modify
manifests, inspect jar contents, or execute artifacts. Actual apply execution
remains future work.

