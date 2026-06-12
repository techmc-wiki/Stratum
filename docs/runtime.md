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

## MCDR RuntimeProfile future shape

The following profile is conceptual and not implemented:

```yaml
id: mcdr-managed
runtime_type: terminal
command: ["python", "-m", "mcdreforged"]
working_dir: "<session_runtime_dir>"
stop_strategy: stdin
stop_stdin_command: "!!MCDR stop"
graceful_stop_timeout: "30s"
force_kill_timeout: "10s"
enabled_by_default: false
```

In this profile, Stratum Agent launches and supervises MCDR. MCDR may expose
plugins, in-game commands, and Minecraft console behavior inside the runtime.
If MCDR exits, hangs, or fails to stop Minecraft, the Agent still owns status
reporting and force-stop behavior.

Lucy remains a non-intrusive dependency, manifest, and lock-management
integration. It is not a process supervisor or session lifecycle controller.
