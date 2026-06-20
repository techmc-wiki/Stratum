# Minecraft Launch Status and Verification

This document records what StratumMC can verify today, what it does not yet
prove, and how to add a real Minecraft launch smoke test without weakening the
Agent runtime ownership boundary.

StratumMC must treat Minecraft startup as an Agent-owned runtime operation. The
Controller may request lifecycle actions and persist metadata, but it must not
own process handles, terminal I/O, Java execution, MCDR execution, or local
runtime files.

## Current Status

The current test suite proves these pieces separately:

1. The Agent can materialize an MCDR-oriented session layout.
2. The Agent can generate `config.yml` with a `start_command` pointing at Java
   and the deployed server jar name.
3. The Agent can start and stop a trusted RuntimeProfile process.
4. The server jar downloader can download and deploy real Vanilla, Fabric, and
   Paper jars.
5. The local MCDR e2e test does not start real Minecraft. It starts a Go test
   helper process that simulates an MCDR runtime.

The important distinction is this:

```text
Current local MCDR e2e:
  materialize layout
  write fake Fabric jar bytes
  generate MCDR config.yml
  start mock helper process
  stop mock helper process

Not yet covered:
  download real Fabric server jar
  launch real Java process
  wait for Minecraft server readiness logs
  stop the real server cleanly
```

So the existing e2e test verifies Stratum's runtime plumbing, not a real
Minecraft boot.

## Verified Tests

Run the local MCDR e2e test:

```powershell
go test -count=1 -v -run TestE2EMCDRSessionMaterializeAndStart ./internal/agent/local
```

Expected result:

```text
PASS
ok   github.com/stratummc/stratum/internal/agent/local
```

What this test proves:

- `MaterializeEnvironment` creates session runtime metadata and MCDR layout.
- Java detection metadata is recorded.
- Server jar deployment metadata is recorded.
- `work/mcdr/config/config.yml` is generated.
- The generated `start_command` contains the selected Java executable and jar
  name.
- The Agent starts and stops a trusted child process through the MCDR
  RuntimeProfile path.

What this test does not prove:

- It does not execute `java -jar`.
- It does not run MCDR itself.
- It does not run Minecraft.
- It does not prove Fabric server boot compatibility.

The mock process is `TestE2EMCDRHelperProcess` in
`internal/agent/local/e2e_test.go`. The fake server jar is written by
`e2eServerJarDeployer` and has source metadata `e2e-test`.

Run all local MCDR e2e tests:

```powershell
go test -count=1 -v -run TestE2EMCDR ./internal/agent/local
```

Run server jar download tests:

```powershell
go test -count=1 -v ./internal/agent/serverjar
```

These tests may need external network access. If Mojang, Fabric, PaperMC, or a
local proxy is unavailable, they can fail for environmental reasons.

## Real Jar Download Coverage

Real download coverage lives in `internal/agent/serverjar/downloader_test.go`.

Important tests:

- `TestDownloadFabric`: downloads a real Fabric server jar.
- `TestDownloadVanilla`: downloads a real Mojang Vanilla server jar.
- `TestDownloadPaper`: downloads a real Paper server jar.
- `TestDeployServers`: deploys a downloaded Fabric jar into a target directory.
- `TestDeployFabricLatestLoader`: resolves and deploys the latest compatible
  Fabric loader for a Minecraft version.

These tests verify download, file existence, size, and SHA-256 metadata. They do
not start Java or Minecraft.

## Runtime Layout

The Agent runtime root contains machine-local session files. With the default
local layout, paths look like this:

```text
<runtime-root>/
  sessions/
    <session-id>/
      config/
        environment-materialization.json
        lucy.yaml
        lucy-lock.yaml
      work/
        mcdr/
          config/
            config.yml
            permission.yml
          server/
            eula.txt
            server.properties
            <server-jar>
          plugins/
          logs/
          tmp/
          venv/
      world/
      logs/
      mods/
      artifacts/
      checkpoints/
      tmp/
```

`environment-materialization.json` is diagnostic metadata. It records selected
environment values, Java detection metadata, server jar deployment metadata,
Lucy metadata, and MCDR materialization metadata.

`work/mcdr/config/config.yml` is the MCDR configuration generated before MCDR is
started. Its `start_command` is a YAML string and may contain escaped quotes,
for example:

```yaml
start_command: "\"/usr/bin/java17\" -jar \"fabric-server-1.17.1-fat.jar\" nogui"
```

Tests should check the semantic fragments they need, such as the Java executable
and jar name, instead of assuming one exact YAML quoting style.

## RuntimeProfile Boundary

Minecraft and MCDR startup must go through trusted Agent RuntimeProfiles.

Rules:

- RuntimeProfiles are local machine-owner configuration.
- Users and Controller requests select a profile by ID only.
- Users must not provide executable paths, shell fragments, arguments, working
  directories, or stop commands.
- The Agent starts child processes with argv arrays, not shell strings.
- The Agent owns PID tracking, stdin/stdout/stderr, logs, stop, force kill,
  crash detection, and future sandboxing.
- MCDR may be a child process supervised by the Agent, but MCDR is not the
  Stratum lifecycle source of truth.
- Lucy resolves and installs dependency artifacts. Lucy must not control Java,
  MCDR, or Minecraft processes.

Example trusted MCDR profile shape:

```json
{
  "runtime_profiles": [
    {
      "id": "mcdr-fabric-1.17",
      "name": "MCDR Fabric 1.17",
      "runtime_type": "mcdr-python",
      "command_argv": ["mcdreforged", "--start"],
      "working_dir": ".",
      "stop_strategy": "stdin",
      "stop_stdin_command": "!!MCDR stop",
      "graceful_stop_timeout": "60s",
      "force_kill_timeout": "15s",
      "log_mode": "combined",
      "enabled": true,
      "readiness_check": {
        "type": "log-pattern",
        "pattern": "Done (",
        "timeout": "180s"
      }
    }
  ]
}
```

This is deployment configuration, not a public API payload.

## Dependency Checklist for Real Launch

A real Minecraft launch smoke test should require explicit opt-in and these
local dependencies:

1. Java compatible with the target Minecraft version.
2. Python compatible with MCDR.
3. `mcdreforged` available, or a tested venv installation path.
4. Network access to Mojang/Fabric/Paper endpoints, unless cache is already
   populated.
5. Optional proxy configuration for restricted networks.
6. A free server port.
7. Enough memory and CPU to boot the target server.

Useful commands:

```powershell
java -version
python --version
mcdreforged --version
```

Proxy environment variables used by current code paths:

```powershell
$env:STRATUM_PROXY = "http://127.0.0.1:10808"
$env:STRATUM_HTTP_PROXY = "http://127.0.0.1:10808"
```

## Proposed Real Minecraft Smoke E2E

The real launch test should be gated so normal local test runs and CI are not
forced to download jars, allocate memory, bind ports, or depend on external
services.

Suggested gate:

```powershell
$env:STRATUM_RUN_REAL_MINECRAFT_E2E = "1"
go test -count=1 -v -run TestRealMinecraftFabricLaunch ./internal/agent/local
```

The test should do this:

1. Skip unless `STRATUM_RUN_REAL_MINECRAFT_E2E=1`.
2. Create a temp runtime root.
3. Use the real Java detector.
4. Use the real server jar deployer for Fabric.
5. Materialize a 1.17.1 Fabric environment.
6. Start a trusted RuntimeProfile that launches real MCDR or a direct Java
   Minecraft runtime.
7. Wait for a readiness log pattern such as `Done (`.
8. Send a graceful stop command.
9. Assert the process exits and temp directories can be cleaned up.

The test should record enough logs to diagnose failure, but it must not print
secrets, proxy credentials, or host-specific private paths beyond normal temp
diagnostics.

## Manual Investigation Flow

Use this flow when diagnosing whether a session actually launched Minecraft.

1. Run the targeted e2e test and inspect metadata:

```powershell
go test -count=1 -v -run TestE2EMCDRSessionMaterializeAndStart ./internal/agent/local
```

2. Check the server jar source in test logs:

```text
serverJarSource:e2e-test
```

If the source is `e2e-test`, the jar is fake and Minecraft was not launched.

3. Check the process command path:

```text
-test.run=TestE2EMCDRHelperProcess
```

If the command targets the test helper, the process is a mock runtime.

4. Check `config.yml`:

```text
work/mcdr/config/config.yml
```

This proves the launch configuration was generated. It does not prove MCDR or
Minecraft consumed it.

5. For a real launch, require logs from the Java/Minecraft process, such as
server bootstrap lines and a readiness message.

## Troubleshooting

### Fabric Download Fails

- Confirm network access to `meta.fabricmc.net`.
- Configure `STRATUM_PROXY` if the local network needs a proxy.
- Retry with `go test -count=1 -v ./internal/agent/serverjar`.

### Java Detection Fails

- Run `java -version`.
- Set `JAVA_HOME` if Java is installed but not discoverable.
- Verify the Java major version matches the target Minecraft version.

### MCDR Executable Is Accidentally Used in Mock Tests

Mock e2e tests should not use a global `mcdreforged` binary. The local e2e mock
manager rejects global MCDR verification so the test remains deterministic.

### Temp Directory Cleanup Fails on Windows

This usually means a child process is still running or holding a file handle.
Always assert `StopSession` errors in tests, and wait for the supervised process
to exit before the temp directory is cleaned up.

### `config.yml` Assertion Fails

Remember that `start_command` is YAML-escaped. Prefer checking required content
fragments instead of matching one exact quoted line.

## Next Atomic Task

Add a gated real Minecraft smoke e2e:

```text
test: add opt-in real Fabric Minecraft launch smoke test
```

The task should be narrow: one environment, one real server jar, one startup
readiness pattern, one graceful stop path. Broader runtime hardening, sandboxing,
port allocation policy, and production orchestration should remain separate
tasks.
