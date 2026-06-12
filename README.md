# StratumMC

StratumMC is a Project/Room-centered collaborative Minecraft technical testing
control plane for invited technical communities. It coordinates shared rooms,
temporary fork sessions, semantic checkpoints, artifact review, and constrained
compute resources while delegating machine-local process supervision to Stratum
Agents. MCDR and Lucy are optional integrations, not the core control plane.

Lifecycle requests are tracked as durable operations with request correlation,
idempotency, timeout status, and audit history. See `docs/operations.md`.

For a safe development runtime, start the HTTP agent and drive a session through
it. The agent supervises only the built-in Go dummy runtime; it does not launch
Minecraft, MCDR, Lucy, shells, or user commands.

```powershell
go run ./cmd/stratum-agent serve --listen 127.0.0.1:8787
go run ./cmd/stratum --data-dir .stratum/process-test --agent-url http://127.0.0.1:8787 sessions start --id demo-session --actor bryan
go run ./cmd/stratum --data-dir .stratum/process-test --agent-url http://127.0.0.1:8787 sessions inspect --id demo-session
go run ./cmd/stratum --data-dir .stratum/process-test --agent-url http://127.0.0.1:8787 sessions logs --id demo-session
```

The current repository is an MVP skeleton. It contains domain logic, in-memory
repositories, durable metadata, lifecycle services, a fake local agent protocol,
integration interfaces, JSON schemas, and a minimal standard-library CLI. It
does not launch Minecraft or MCDR yet. The development HTTP Agent supervises a
Go-native dummy runtime only. Future MCDR support will run as a trusted child
RuntimeProfile under Agent process supervision.

The Agent exposes its enabled profiles with `stratum agents runtime-profiles
--id local`. Session start/restart accepts `--runtime-profile dummy-process`;
the CLI never accepts executable or shell command input.

The Agent contains a managed terminal executor for trusted profiles. It uses
argv-only `os/exec`, constrains working directories beneath an Agent runtime
root, captures bounded stdout/stderr, and enforces graceful/force stop timeouts.
No terminal, MCDR, or Minecraft profile is enabled in the default registry.

## Quick start

```bash
go test ./...
go run ./cmd/stratum --data-dir .stratum/data projects create --id demo --name "Demo Project"
go run ./cmd/stratum --data-dir .stratum/data rooms create --id demo-room --project demo --name "Demo Room"
go run ./cmd/stratum --data-dir .stratum/data sessions create --id demo-session --project demo --room demo-room
```

See [docs/architecture.md](docs/architecture.md), [docs/runtime.md](docs/runtime.md),
[docs/storage.md](docs/storage.md), [docs/mvp.md](docs/mvp.md), and
[docs/security.md](docs/security.md) for design details and safety rules.
Development and review conventions are in [docs/workflow.md](docs/workflow.md).

## HTTP agent development

```bash
# Terminal 1
go run ./cmd/stratum-agent serve --listen 127.0.0.1:8787

# Terminal 2
go run ./cmd/stratum --agent-url http://127.0.0.1:8787 agents inspect --id local
```

Add matching `--token` and `--agent-token` flags to enable the local shared-token
placeholder. The HTTP Agent uses the dummy process supervisor and never starts
Minecraft, MCDR, Lucy, a shell, or a user-provided command.
A collaborative Minecraft technical testing control plane.
