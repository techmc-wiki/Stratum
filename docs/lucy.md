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

## Future Adapter Paths

Possible implementations include:

1. a Lucy CLI JSON adapter,
2. a Lucy Go package adapter,
3. a Stratum Artifact provider implemented inside Lucy,
4. a Lucy dry-run plan consumed and validated by Stratum.

Future provider sources may include `modrinth`, `curseforge`, `github-release`,
`local-file`, `stratum-artifact`, and deployment-specific custom sources. These
names are data values, not dependencies on Lucy provider implementations.
