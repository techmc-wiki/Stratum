# Lucy Adapter Boundary

Lucy is a non-intrusive environment dependency and consistency tool. It does
not own Stratum Sessions, supervise runtime processes, or start and stop JVMs.
Stratum Agent remains responsible for the outer process lifecycle, including
any future MCDR or Minecraft child process.

## Stratum Contract

`internal/integration/lucy` defines Stratum-owned interfaces and DTOs for four
operations:

* capability discovery,
* environment planning,
* environment locking,
* environment status checks.

The contract describes package requests, verified local Artifact references,
planned actions, locks, and drift status. It deliberately does not expose Lucy
Provider names as Go types, Lucy internal package structures, or a final Lucy
manifest schema. `NoopAdapter` supplies deterministic empty responses for tests
and future wiring without performing I/O.

## Boundary Rules

Stratum adapters must not make Controller or Agent code depend on Lucy internal
Provider or package types. The current boundary does not:

* invoke the Lucy CLI,
* import Lucy packages,
* read or write `lucy.yaml` or `lucy-lock.yaml`,
* resolve packages or download files,
* install dependencies,
* mutate runtime directories,
* start MCDR, Minecraft, or another JVM.

Plan actions are descriptive data. A future implementation must keep execution
behind explicit Agent-owned filesystem and runtime boundaries.

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

## Future Adapter Paths

Possible implementations include:

1. a Lucy CLI JSON adapter,
2. a Lucy Go package adapter,
3. a Stratum Artifact provider implemented inside Lucy,
4. a Lucy dry-run plan consumed and validated by Stratum.

Future provider sources may include `modrinth`, `curseforge`, `github-release`,
`local-file`, `stratum-artifact`, and deployment-specific custom sources. These
names are data values, not dependencies on Lucy provider implementations.
