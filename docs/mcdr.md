# MCDR Bridge

The MCDR bridge (`internal/agent/mcdrbridge`) defines a planning-only
contract for MCDR child runtime integration. It formalises launch plan
construction, validation, and inspection as metadata-only operations.
The bridge does not start MCDR, does not start Minecraft, and does not
install any dependencies. MCDR remains a future Agent-supervised child
RuntimeProfile.

## Ownership

- The Agent owns process supervision for all runtimes including MCDR.
- The Controller owns authoritative Session state and metadata.
- The MCDR bridge is a planning layer only; it does not execute anything.
- MCDR does not own Session lifecycle. The Controller does not call MCDR
  directly for start, stop, or restart operations.
- Future MCDR RuntimeProfile launch will be implemented as a new atomic
  task. The launch plan defined here is the planning contract that a
  future RuntimeProfile will read.

## Bridge Interface

```go
type MCDRBridge interface {
    BuildLaunchPlan(ctx context.Context, req BuildLaunchPlanRequest) (*LaunchPlan, error)
    ValidateLaunchPlan(ctx context.Context, plan LaunchPlan) error
    InspectLaunchPlan(ctx context.Context, sessionID string) (LaunchPlanStatus, error)
}
```

- `BuildLaunchPlan` derives session and MCDR runtime layouts from the
  bridge's runtime root, constructs a `LaunchPlan` from the request,
  validates it, ensures MCDR directories exist, and atomically writes
  the launch-plan manifest.
- `ValidateLaunchPlan` checks struct fields only (no filesystem access).
- `InspectLaunchPlan` reads and validates the persisted manifest.
  It never modifies the file.

## BuildLaunchPlanRequest

```go
type BuildLaunchPlanRequest struct {
    SessionID         string
    RuntimeRoot       string
    SessionRoot       string
    WorkDir           string
    ConfigDir         string
    LogsDir           string
    EnvironmentID     string
    MinecraftVersion  string
    JavaVersion       string
    LoaderType        string
    LoaderVersion     string
    ServerCore        string
    RuntimeProfileID  string
    MCDRRequired      bool
    CarpetRequired    bool
    Metadata          map[string]string
}
```

The bridge derives canonical session-relative paths from the runtime
root and SessionID. Request-level path fields are reserved for future
override support; path derivation uses the `process.SessionRuntimeLayout`
and `process.MCDRRuntimeLayout` helpers for safety.

## Launch Plan Manifest

`LaunchPlan` is the full MCDR launch plan DTO persisted to disk. It
records environment identity, canonical MCDR paths, launch command,
stop strategy, preconditions, plan status, and user metadata.

The manifest is written at `work/mcdr/mcdr-launch-plan.json` under the
session runtime root. This is consistent with the existing MCDR layout
manifest (`mcdr-layout.json`) and config stub manifest
(`mcdr-config-stub.json`).

### Launch Plan Fields

| Field | Description |
|---|---|
| `session_id` | Session identity |
| `environment_id` | Environment identity |
| `minecraft_version` | Minecraft version |
| `java_version` | Java version |
| `loader_type` | Loader (e.g. fabric) |
| `loader_version` | Loader version |
| `server_core` | Server core identifier |
| `runtime_profile_id` | Expected RuntimeProfile |
| `status` | planned / missing_layout / invalid / unsupported |
| `mcdr_root` | MCDR root within session |
| `config_dir` | MCDR config directory |
| `plugins_dir` | MCDR plugins directory |
| `server_dir` | Minecraft server directory |
| `logs_dir` | MCDR logs directory |
| `server_work_dir` | Minecraft working directory |
| `planned_config_path` | Path for future config.yml |
| `planned_server_properties_path` | Path for server.properties |
| `planned_eula_path` | Path for eula.txt |
| `command` | Launch command (executable + argv) |
| `stop` | Stop strategy configuration |
| `preconditions` | Check statuses (python, mcdr, server jar, eula) |
| `issues` | Validation issues |
| `planned_at` | Plan creation timestamp |
| `notes` | Planning notes |
| `metadata` | Arbitrary key-value metadata |

### Launch Command

```go
type LaunchCommand struct {
    Executable string   `json:"executable"`
    Argv       []string `json:"argv"`
    WorkingDir string   `json:"working_dir"`
    EnvKeys    []string `json:"env_keys,omitempty"`
}
```

The command is always argv-based (never a shell string). `EnvKeys`
records required environment variable names without values; secrets
are never stored in the launch plan.

Default command for the planning stub: `executable: python`,
`argv: ["-m", "mcdreforged"]`.

### Stop Strategy

```go
type StopStrategy string

const (
    StopStrategyStdin  StopStrategy = "stdin"
    StopStrategySignal StopStrategy = "signal"
    StopStrategyNone   StopStrategy = "none"
)
```

Default strategy is `stdin` using the `!!MCDR stop` convention, but
the `stdin_command` field is left empty in the planning stub because
the actual stop command is a RuntimeProfile deployment concern.

### Preconditions

```go
type PreconditionStatus string

const (
    PreconditionUnknown    PreconditionStatus = "unknown"
    PreconditionRequired   PreconditionStatus = "required"
    PreconditionNotChecked PreconditionStatus = "not_checked"
)
```

All preconditions default to `not_checked`. The bridge does not probe
for Python, MCDR, server jars, or eula files. These are recorded as
unknown until a real installation check is implemented.

## Launch Plan Status

```go
type LaunchPlanStatus struct {
    SessionID string
    Exists    bool
    Valid     bool
    Status    string
    Path      string
    Issues    []string
    CheckedAt time.Time
    Summary   string
}
```

`InspectLaunchPlan` returns this struct. It reads the manifest from
disk, validates the JSON, checks session ID match, and reports path
safety issues.

## Validation Rules

`ValidateLaunchPlan` enforces the following rules on the plan struct
without filesystem access:

1. SessionID must be non-empty and path-safe (`[a-zA-Z0-9\-_.]`).
2. All path fields must be runtime-relative:
   - No backslashes.
   - No absolute paths.
   - No Windows volume prefixes (e.g. `C:`).
   - Clean canonical form (no `.`, `..`, or `../` prefix).
3. Command must be argv-based:
   - `executable` is required.
   - `argv` must have at least one element.
   - No newlines in any argv entry.
4. Stop strategy must be one of `stdin`, `signal`, `none`.
5. Precondition values must be one of `unknown`, `required`,
   `not_checked`.
6. Plan status must be one of `planned`, `missing_layout`, `invalid`,
   `unsupported`.

`BuildLaunchPlan` additionally validates at the filesystem level:
- MCDR root must exist (or be creatable).
- Manifest path must stay within the MCDR root.
- Atomic write uses temp-file + rename.

## Integration with Existing MCDR Layout

The MCDR bridge builds on the existing MCDR runtime layout:

- `process.SessionRuntimeLayout` provides session root, work, logs,
  config, artifacts, checkpoints, and tmp directories.
- `process.MCDRRuntimeLayout` derives MCDR-specific paths under
  `work/mcdr/`: config, plugins, server, logs, tmp.
- `BuildLaunchPlan` calls `sessionLayout.MCDR()` to derive canonical
  MCDR paths and `mcdrLayout.Create()` to ensure directories exist.

The launch-plan manifest (`mcdr-launch-plan.json`) is written to the
same directory as the existing `mcdr-layout.json` and
`mcdr-config-stub.json`. These three files serve different purposes:

| File | Purpose |
|---|---|
| `mcdr-layout.json` | Records prepared MCDR directory paths |
| `mcdr-config-stub.json` | Records planned config location metadata |
| `mcdr-launch-plan.json` | Records full launch plan for a future RuntimeProfile |

## Idempotency

- Re-running `BuildLaunchPlan` with the same request overwrites the
  existing manifest with identical content.
- `InspectLaunchPlan` is read-only and never modifies the manifest.
- Directory creation uses `MkdirAll` and tolerates existing directories.

## What the Bridge Does NOT Do

- Does not start MCDR or Minecraft.
- Does not install Python, pip, or MCDR dependencies.
- Does not generate `config.yml`, `server.properties`, or `eula.txt`.
- Does not download server jars or mod files.
- Does not call Lucy for dependency resolution.
- Does not change Session lifecycle state.
- Does not change Controller authoritative metadata.
- Does not implement RuntimeProfile mcdr-managed startup.
- Does not write files outside the session runtime root.

## Future Work

- Real MCDR RuntimeProfile that reads the launch plan and starts MCDR
  as a child process.
- Precondition probes that check for Python, MCDR, server jar, and
  eula file availability.
- Environment variable population from the launch plan's `EnvKeys`.
- CLI and HTTP endpoints for build/inspect operations.
- Validation of launch plan against the enabled RuntimeProfile.
