# Lucy Integration

Lucy is integrated as a direct Go dependency (`github.com/mclucy/lucy`) and provides non-intrusive environment dependency resolution and consistency checking. Lucy does not own Stratum Sessions, supervise runtime processes, or start and stop JVMs. Stratum Agent remains responsible for the outer process lifecycle, including any future MCDR or Minecraft child process.

## Integration Model

StratumMC calls Lucy library functions directly through Go function calls in production deployments. The integration boundary under `internal/integration/lucy` provides:

* `Adapter` interface — type-safe contract for Environment and Artifact dependency operations
* `EmbeddedAdapter` — production implementation calling Lucy Go APIs directly
* `CLIAdapter` — fallback implementation shelling out to `lucy` command (backup only)
* `NoopAdapter` — no-op stub for tests or environments without Lucy

### Dependency Configuration

Lucy is declared in `go.mod`:

```go
require github.com/mclucy/lucy v0.0.0-20260617080255-b2539d110491 // indirect

replace github.com/mclucy/lucy => ./tools/lucy
```

The local replace directive points to Lucy's source tree under `tools/lucy` for development. Production builds may point to a published Lucy version or vendored copy.

## Stratum Contract

The Adapter interface defines four core operations:

* capability discovery,
* environment planning,
* environment locking,
* environment status checks.

The contract describes package requests, verified local Artifact references, planned actions, locks, and drift status. It deliberately does not expose Lucy Provider names as Go types or Lucy internal package structures through Stratum's stable domain interfaces.

## Boundary Rules

Stratum adapters must not make Controller or Agent domain code depend on Lucy internal Provider or package types. The integration boundary:

* translates Lucy resolution results into Stratum Artifact and Environment DTOs,
* validates Lucy outputs before consuming them,
* isolates Lucy dependency changes from Stratum's core domain model,
* preserves Agent control over filesystem mutation and runtime execution.

Lucy provides dependency planning data. Agent code executes materialization, file writes, and verification steps using Stratum-owned logic.

## Adapter DTO Validation

Stratum validates Lucy adapter request and response DTOs before future adapter
wiring. Validation is structural and safety-focused: it checks required IDs and
fields, paired payload metadata, non-negative sizes, supported action values,
and cross-platform relative paths for runtime targets.

Validation does not resolve packages, download files, inspect dependency
sources, or read Lucy manifests. Real Lucy adapters must produce Stratum-owned
DTOs that pass these rules before their results are consumed.

## Adapter Error Classification

Stratum classifies Lucy adapter failures using Stratum-owned error codes
independent from Lucy internal implementation. Real Lucy adapters must map
provider errors, CLI failures, and package resolution problems into these codes
for operator diagnostics and future retry policy.

Defined error codes:

* `invalid_request`: malformed request or unsupported parameters (non-retryable).
* `validation_failed`: failed DTO or constraint validation (non-retryable).
* `unsupported_capability`: operation not supported by adapter (non-retryable).
* `provider_unavailable`: dependency source unreachable (retryable).
* `package_not_found`: package or version not found (non-retryable).
* `version_conflict`: dependency resolution conflict (non-retryable).
* `lock_conflict`: lock and manifest mismatch (non-retryable).
* `checksum_mismatch`: payload verification failure (non-retryable).
* `io_error`: filesystem or network I/O failure (context-dependent retryability).
* `timeout`: operation timeout (retryable).
* `cancelled`: operation cancelled (retryable).
* `external_command_failed`: CLI or external tool failure (context-dependent
  retryability).
* `internal_error`: adapter internal failure or unknown error (retryable).

Error classification does not execute Lucy, resolve packages, download files, or
alter runtime behavior. Future Controller and Agent logic may consume these
codes for retry decisions, audit logging, and operator alerting.

## CLI JSON Adapter Stub

`CLIAdapter` is a provisional Stratum-owned boundary for communicating with Lucy
via JSON DTOs. It does not depend on Lucy internal Go packages or Provider
implementations. Real Lucy CLI command shape may evolve; the adapter invokes
Lucy with operation-specific arguments (`capabilities`, `plan_environment`,
`lock_environment`, `check_status`) and `--json` flag.

The adapter serializes request DTOs as JSON to stdin, reads JSON response from
stdout, decodes into Stratum-owned DTOs, validates decoded DTOs, maps
process/JSON/validation errors to AdapterError codes, enforces timeout and max
output size, and uses argv directly without shell execution.

Tests use fake `CommandRunner` interface; real Lucy execution is not required.
No Lucy binary is installed or invoked during tests. The adapter does not
resolve packages, download files, write manifests, or alter runtime behavior.

Future implementations may replace CLIAdapter with Lucy Go package adapter or
other integration strategies without changing Stratum Controller or Agent
behavior.

## Stratum Metadata to Lucy DTO Mapping

Stratum maps Environment and Artifact metadata into Lucy adapter DTOs via the
`lucybridge` package. This mapping is local and non-executing. It does not call
Lucy, resolve packages, download files, or write manifests.

`EnvironmentToSpec` maps Stratum Environment to `lucy.EnvironmentSpec`:
environment_id, minecraft_version, java_version, loader_type, loader_version,
server_core, carpet_required, mcdr_required, runtime_profile_id, and safe
metadata subset. Mapped spec is validated before returning.

`ArtifactToLocalRef` maps Stratum Artifact to `lucy.LocalArtifactRef`:
artifact_id, payload_algorithm, payload_hash, payload_size, artifact_type,
runtime_name, and safe metadata subset. Runtime_name traversal is rejected via
validation. Mapped ref is validated before returning.

PackageRef population and real dependency resolution remain future work. Empty
Packages slice is acceptable; future Lucy manifest or provider work will
populate package dependencies.

LocalArtifactRef lets future Lucy providers reason about already-approved
Stratum artifacts without owning Artifact lifecycle, blob storage, or approval
workflow.

## Safe Exec CommandRunner

`ExecCommandRunner` is a generic bounded process runner for future Lucy CLI
adapters. It implements the `CommandRunner` interface using `os/exec.CommandContext`.
It uses argv directly without shell execution, captures bounded stdout and
stderr separately, supports timeout and cancellation via context, enforces max
output bytes per stream, returns exit code and timeout status in `CommandResult`,
and respects working directory and environment when explicitly provided.

Security boundaries: empty command path rejected, no shell parsing, arguments
passed literally to child process, output size bounded, context cancellation
respected, no automatic logging of stdout/stderr or secrets.

Tests use Go test helper process pattern for cross-platform behavior without
requiring real Lucy binary. Context cancellation behavior is platform-dependent
and tested manually.

`ExecCommandRunner` does not require or invoke real Lucy unless explicitly
configured later. It does not resolve packages, download files, write manifests,
or alter runtime behavior by itself. It is a safe reusable building block for
CLIAdapter or future process-based integrations.

## CLIAdapter Runner Configuration

`CLIAdapter` constructor accepts `CLIAdapterOptions` with optional `Runner` or
`UseExec` flag. If `Runner` is provided, it is used directly. If `Runner` is
nil but `UseExec` is true, `ExecCommandRunner` is automatically instantiated.
If neither `Runner` nor `UseExec` is provided, constructor returns
`invalid_request` error.

`CommandPath` is always required. Empty command path fails with `invalid_request`.

Stratum does not auto-discover or auto-run Lucy from PATH. Real Lucy CLI
integration requires explicit `CommandPath` configuration and remains opt-in
future work.

This wiring still does not resolve packages, download files, write manifests,
or alter runtime behavior. It only wires the process runner abstraction for
future Lucy CLI integration.

## Embedded Go Adapter

EmbeddedAdapter is the production implementation that calls Lucy library functions directly within the Stratum process. It wraps an `EmbeddedBackend` that provides Plan, Lock, Status, and Capabilities methods using Lucy's Go APIs.

EmbeddedAdapter:

* validates all requests before calling Lucy,
* validates all responses from Lucy before returning to Stratum,
* translates Lucy errors into Stratum AdapterError types,
* preserves Stratum's DTO boundary without exposing Lucy internal types.

This approach eliminates subprocess overhead and JSON serialization costs. EmbeddedAdapter performs no CLI invocation and does not spawn external processes.

CLIAdapter remains available as a fallback for environments where Lucy library integration is unavailable or for debugging resolution behavior outside the Stratum process.

## Agent Environment Materialization Wiring

Agent environment materialization accepts an optional Lucy Adapter. The Supervisor stores a `lucyAdapter` field initialized to NoopAdapter by default. `SetLucyAdapter(adapter)` allows injecting EmbeddedAdapter, CLIAdapter, or nil (which defaults to NoopAdapter).

MaterializeEnvironment records Lucy adapter metadata in both the result Metadata map and the persisted environment-materialization.json manifest:

* `lucyAdapterMode`: "noop", "embedded", "cli", or "unknown"
* `lucyResolutionStatus`: "not_requested", "success", or error code
* `lucyAdapterConfigured`: "true" or "false"

Current wiring is minimal. Future work may call PlanEnvironment or LockEnvironment during Environment materialization to resolve dependencies and generate lock files.

## Provider Sources

Lucy supports multiple dependency providers:

* `modrinth` — Modrinth mod repository
* `curseforge` — CurseForge mod repository
* `maven` — Maven artifact repositories
* `github-release` — GitHub release assets
* `url` — direct URL download
* `local-file` — local filesystem references
* `stratum-artifact` — future Stratum Artifact provider

These are data values in Lucy's resolution model. Stratum does not depend on Lucy provider implementations as Go types.
