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

## Future Adapter Paths

Possible implementations include:

1. a Lucy CLI JSON adapter,
2. a Lucy Go package adapter,
3. a Stratum Artifact provider implemented inside Lucy,
4. a Lucy dry-run plan consumed and validated by Stratum.

Future provider sources may include `modrinth`, `curseforge`, `github-release`,
`local-file`, `stratum-artifact`, and deployment-specific custom sources. These
names are data values, not dependencies on Lucy provider implementations.
