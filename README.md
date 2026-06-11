# StratumMC

StratumMC is a Project/Room-centered collaborative Minecraft technical testing
control plane for invited technical communities. It coordinates shared rooms,
temporary fork sessions, semantic checkpoints, artifact review, and constrained
compute resources while keeping Minecraft runtime control behind MCDR and
environment consistency behind non-intrusive Lucy integrations.

The current repository is an MVP skeleton. It contains domain logic, in-memory
repositories, service boundaries, integration interfaces, JSON schemas, and a
minimal standard-library CLI. It does not launch Minecraft yet.

## Quick start

```bash
go test ./...
go run ./cmd/stratum --data-dir .stratum/data projects create --id demo --name "Demo Project"
go run ./cmd/stratum --data-dir .stratum/data rooms create --id demo-room --project demo --name "Demo Room"
go run ./cmd/stratum --data-dir .stratum/data sessions create --id demo-session --project demo --room demo-room
```

See [docs/architecture.md](docs/architecture.md), [docs/storage.md](docs/storage.md),
[docs/mvp.md](docs/mvp.md), and [docs/security.md](docs/security.md) for design
details and safety rules.
A collaborative Minecraft technical testing control plane.
