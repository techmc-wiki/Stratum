# AGENTS.md

## Project Overview

StratumMC is a high-end collaborative Minecraft technical testing control plane
for invited advanced players, especially TMC / redstone / world-mechanics
researchers. GTMC is the first deployment community.

It is **not** a generic Minecraft hosting panel. The model is
Project → Room → Session → Checkpoint, with Fork Sessions for risky experiments,
Artifacts for uploadable assets, Environments for runtime templates, and a
resource-aware scheduler that treats compute as scarce.

It is a **CLI-first** tool. The `stratum` CLI is the primary UX and must be
complete, scriptable, and self-sufficient. A future Web UI is a convenience
layer, never a substitute for CLI completeness.

Current build state (per `docs/status.md`, 2026-06-20):

- Phases 1–4 and 6a/6c are **complete** (domain models, storage, operations,
  controller, agent, Lucy, MCDR, Java detection, server-jar provisioning, world
  checkpoints, 3 environments, multi-agent registry, Docker Compose).
- Phase 5 (Web UI) is **skipped**.
- Phase 6b (AuthN/AuthZ, RBAC) is **not started** — only shared-token auth exists.
- Real Minecraft boot via MCDR is **not yet validated end-to-end**; tests use
  helper-process stubs (see `MINECRAFT_LAUNCH.md`).

## Project Skills

This repo ships Golang skills via `skills-lock.json`. If your agent client does
not surface them, run:

```bash
npx skills -y experimental_install
```

Then ask the human to restart the client. Skills cover Go conventions for
testing, error handling, concurrency, lint, security, and more — load the
matching skill before non-trivial Go work.

## Primary Language

- **Go** for everything load-bearing: controller, agent, CLI, domain, services,
  repositories, scheduler, runtime integration.
- **Python** only for optional tooling, future MCDR plugins, migration scripts,
  world/NBT helpers, and admin utilities. Never implement core controller or
  agent in Python.

Module: `github.com/stratummc/stratum`. Toolchain: Go 1.25+ (local 1.26 is fine).
Lucy is a direct Go dependency with a `replace` directive pointing at
`./tools/lucy` (`github.com/mclucy/lucy`).

## Setup Commands

```bash
# Download Go module dependencies (Lucy is vendored via ./tools/lucy)
task deps         # or: go mod download

# Format all Go packages
task fmt          # or: go fmt ./...

# Vet
task vet          # or: go vet ./...

# Full test suite
task test         # or: go test -count=1 ./...

# Build host-native binaries into ./dist/local
task build

# Linux amd64 cross-compile into ./dist/linux-amd64 (CGO disabled)
task build:linux-amd64

# Mirror GitHub Actions: deps → vet → test → linux-amd64 build
task ci

# Remove build artifacts
task clean
```

Binaries produced by `task build`: `stratum`, `stratum-agent`,
`stratum-controller`, and `lucy` (the Lucy helper CLI built from `cmd/lucy`).

Running services locally:

```bash
# Controller (default :8080, filesystem data dir)
dist/local/stratum-controller serve --listen :8080 --data-dir ./data

# Agent (auto-registers with controller, 30s heartbeat)
dist/local/stratum-agent \
  --controller-url http://127.0.0.1:8080 \
  --listen :8787 \
  --runtime-root ./runtime \
  --data-dir ./data
```

Smoke tests should use a throwaway data dir under `.stratum/` and stop any
background Agent when finished — see `docs/workflow.md`.

## Testing Instructions

- Standard Go testing. Run everything: `go test -count=1 ./...`.
- Prefer focused package tests during development:
  `go test -count=1 ./internal/checkpoint/...`
- Run a single test: `go test -count=1 -run TestName ./internal/...`
- Tests live alongside source as `*_test.go`. CLI tests are concentrated in
  `internal/cli/cli_test.go` (~95 KB).
- Coverage is not gated by CI today; still add tests when changing core logic.
- Minimum areas that must stay green:
  - resource policy decisions,
  - session state transitions,
  - checkpoint metadata creation,
  - artifact hash metadata,
  - permission checks,
  - audit event creation,
  - Lucy install/verify flows.

The `test-*.ps1` scripts at the repo root are PowerShell e2e harnesses for
runtime plumbing validation on Windows hosts. They are not part of the Go test
suite.

## Code Style

- Go standard formatting: `gofmt` / `go fmt ./...` before every commit.
- Clear module boundaries over clever abstractions.
- Domain logic stays independent from process execution. Never import `os/exec`
  or runtime packages from `internal/domain/*`.
- No hard-coded deployment paths in domain models.
- Typed constants/enums for domain state — no magic strings.
- Explicit, testable session state transitions.
- Structured errors with actionable messages (see `internal/errors/` and
  `internal/stratumerr/`).
- No silent error swallowing.
- No global mutable state unless clearly isolated.
- Prefer the standard library. Add a dependency only when it clearly improves
  maintainability. The current third-party surface is intentionally small:
  `spf13/cobra`, the `charm.land/*` TUI stack, `mclucy/lucy`.
- TODO comments are expected at external integration boundaries.

## Build and Deployment

- Build outputs land in `dist/local` (host) or `dist/linux-amd64` (CI target).
- Optional Docker deployment configs live in `deploy/docker/`:
  `Dockerfile.controller`, `Dockerfile.agent`, and `docker-compose.yml`. These
  are provided as a convenience — Stratum is not coupled to Docker.
- `.env.example` documents required env vars. Never commit real secrets.
- GitHub Actions workflow lives in `.github/workflows/` and runs the equivalent
  of `task ci`.

## Repository Structure

```text
cmd/
  stratum/            CLI entrypoint (cobra-based)
  stratum-controller/ controller serve command
  stratum-agent/      agent daemon
  lucy/               lucy helper CLI

internal/
  domain/             project, room, session, checkpoint, artifact,
                      environment, resourcepolicy, audit, operation,
                      runtimeobservation, artifactapply, artifactstaging
  service/            projectsvc, roomsvc, sessionsvc, checkpointsvc,
                      artifactsvc, schedulersvc, permissionsvc, ...
  repository/         memory + filesystem backing stores
  storage/            filesystem store, content-addressed blob storage,
                      testdata
  controller/         HTTP API, agentregistry, capability-based routing
  agent/              httptransport, process, mcdr, mcdrbridge,
                      runtimeprofile, serverjar, serverproperties,
                      worldcheckpoint, java, files, local, resource, python
  cli/                cobra commands + handlers (one file per resource type)
  integration/
    lucy/             EmbeddedAdapter + CLI adapter, validation, install,
                      real_backend (production), noop (testing)
    mcdr/             MCDR runtime adapter
  config/ errors/ idgen/ stratumerr/ util/ observation/
  reconcile/ worldprofile/ permission/

schemas/              JSON Schemas (artifact, checkpoint, environment,
                      operation, project, resource-policy, room, session,
                      audit-event, runtime-observation, artifact-apply-plan,
                      artifact-staging-plan)
runtime-profiles/     fabric-latest.json, forge-1.12.json, mcdr-fabric-1.17.json
manifests/            lucy.yaml base mod sets for the three bundled envs
docs/                 architecture, runtime, agent, mcdr, lucy, checkpoints,
                      world-profile, operations, storage, security, mvp,
                      workflow, status, routemap, sub-architecture,
                      lucy-integration, cli-reference
docs/environments/    environment definitions consumed by MaterializeEnvironment
docs/workflows/       end-to-end usage examples
plugins/mcdr/         future MCDR plugin home (README only today)
tools/lucy/           vendored Lucy source (replace directive target)
tools/python/         optional Python helpers
```

Boundary rule: anything under `internal/domain/` must not import anything under
`internal/agent/`, `internal/controller/`, `internal/cli/`, or `os/exec`. Domain
is pure data and rules.

## Core Domain Model

Treat these as load-bearing vocabulary across code, CLI, schemas, and docs:

- **Project** — long-term collaboration unit. Owns members, rooms, sessions,
  artifacts, checkpoints, permissions, audit history.
- **Room** — collaborative workspace inside a project; usually maps to one
  shared Session. Examples: `1.12-main-lab`, `1.17-flat-testing`.
- **Session** — a runnable Minecraft server instance. Types: `shared`, `fork`,
  `private`, `review`, `archived`. Default behavior favors shared collaboration
  over unlimited private instances.
- **Checkpoint** — a semantic experiment snapshot. Not a plain zip backup:
  records world state reference, environment reference, Lucy lock hash,
  artifacts/mods, server configs, Carpet rules, seed and generator settings,
  source room/session, creator, notes, operation history. Dangerous operations
  create a pre-operation checkpoint first.
- **Artifact** — uploaded file or reusable asset (`.jar` mod, datapack, MCDR
  plugin, config preset, Carpet rule preset, schematic/litematic, world
  archive). Must carry uploader, hash, compatibility metadata, approval status,
  usage records, creation time. URL-mixin source compilation is **not**
  supported.
- **Environment** — Minecraft runtime template: version, Java version, loader,
  server core, MCDR config, Carpet type, base mod set, Lucy manifest, Lucy lock.
  Bundled envs: 1.12 Forge (Java 8), 1.17 Fabric (Java 17), Latest Fabric
  (Java 21, auto-resolved via Mojang manifest polled every 6h).
- **ResourcePolicy** — global / per-project / per-user limits: max running
  sessions, idle timeout, fork-session TTL, review-session limits, queueing
  and denial reasons.

## Architectural Boundaries

### Runtime Ownership

- **Stratum Agent** owns the outer process and terminal lifecycle for every
  runtime: start, stop, restart, force-terminate, stdin/stdout/stderr, logs,
  PID and exit code observation, crash detection, resource observation, and
  future sandboxing.
- **Controller** is the source of truth for Project / Room / Session / Operation
  metadata, resource decisions, permissions, audit history, checkpoint
  metadata, and artifact metadata. Agents report observations but **must not**
  directly mutate Controller repositories or authoritative Session state.

### MCDR

MCDR is an *optional child runtime*, never the lifecycle source of truth. It
may be launched by the Agent as a child process via a trusted RuntimeProfile.
The Controller must never call MCDR directly as its start/stop/restart manager.
The Agent must be able to observe MCDR logs/exit-status and stop or restart the
outer runtime even when MCDR or Minecraft is unresponsive.

### Lucy

Lucy is integrated as a direct Go dependency (`internal/integration/lucy`,
backed by `./tools/lucy`). Production deployments use `EmbeddedAdapter` to call
Lucy Go APIs in-process. Lucy provides package reference parsing, provider
routing (Modrinth / CurseForge / Maven / URL), dependency closure resolution,
version conflict solving, download metadata + checksums, lock file generation,
and cache-aware download planning.

Lucy must **not** be treated as a JVM process manager, live server controller,
MCDR replacement, session scheduler, or runtime lifecycle owner.

### RuntimeProfile vs Environment

- **RuntimeProfile** describes *how* a Session executes (process layout, ports,
  MCDR config, terminal vs MCDR supervision, Java version). Stored as
  declarative JSON in `runtime-profiles/`. No shell commands are allowed in a
  profile.
- **Environment** describes *what* Minecraft runtime is required (MC version,
  loader, server core, base mods, Lucy manifest). Stored in
  `docs/environments/` and `manifests/`.

### Uploaded Jars

Uploaded `.jar` files are potentially arbitrary code execution. Never allow
them to affect base worlds, other users' sessions, unrelated projects,
controller files, checkpoint storage, or host-level secrets. Artifacts require
metadata + SHA-256, an approval workflow, and are gated from shared sessions
until approved. Design for future sandboxing.

## Security Rules (Hard)

1. Base worlds are immutable / read-only.
2. Shared rooms require stricter permissions than private/fork sessions.
3. Fork sessions are temporary and resource-controlled.
4. Dangerous operations create a checkpoint first (enforced via
   `--pre-op-checkpoint` on `sessions restart` and `artifacts apply execute`).
5. Uploaded jars must have metadata + hash records (SHA-256, content-addressed
   blob storage).
6. Unapproved artifacts must not attach to shared sessions.
7. Review sessions must be isolated.
8. Lucy must not be used to control the JVM runtime.
9. Stratum Agent owns runtime supervision; MCDR stays an optional child
   RuntimeProfile behind Agent interfaces.
10. Never store secrets in committed files. Use `.env` (gitignored) for local
    passphrases.
11. Uploaded artifacts must not affect base worlds or unrelated sessions.
12. No URL-mixin source compilation.
13. RuntimeProfile is declarative JSON only — no embedded shell commands.
14. World zip restore must defend against zip-slip (symlinks, `..`, absolute
    paths). The existing `worldcheckpoint` restore path already does this.

## Non-Goals

Do not add these unless explicitly requested:

- automatic URL mixin source compilation,
- automatic merging of forked Minecraft worlds back into shared rooms,
- generic game-panel features unrelated to the collaboration/testing model,
- production-grade billing or multi-tenant hosting,
- public registration,
- broad modpack hosting marketplace features,
- core controller / agent implementation in Python,
- modern Forge (1.13+) or NeoForge before Phase 7,
- LiteLoader 1.12 before Phase 7,
- cross-agent world restore before ownership semantics are settled.

## Atomic Change / Commit Policy

One Codex task should normally correspond to one narrow implementation goal.
Prefer small, reviewable changes.

Do **not** mix unrelated changes:

- runtime code + docs rewrite + CLI redesign + schema migration,
- feature implementation + broad refactor,
- bugfix + architecture rename.

If a task requires multiple logical steps, split into clear phases. When
commits are allowed, create atomic commits that each compile and pass relevant
tests. Use a consistent prefix:

`docs:`, `domain:`, `storage:`, `lifecycle:`, `agent:`, `runtime:`, `cli:`,
`test:`, `refactor:`, `chore:`.

Additional rules:

- Do not touch unrelated files just to reformat or reword them.
- Do not rewrite docs wholesale unless the task is specifically documentation
  alignment.
- Do not update `AGENTS.md` during feature work unless project rules themselves
  need to change.
- Do not bundle architecture changes with implementation unless explicitly
  requested.
- Prefer focused tests for the changed module; run the full suite before
  finishing when practical.
- Always summarize changed files grouped by logical commit.

If actual git commits cannot be created, summarize the work as **proposed
atomic commits** in the final response.

## Secrets / Commit Passphrase

If git commit requires an SSH or GPG passphrase, load it from local `.env`.
Never print, echo, log, commit, or expose the passphrase. Never modify or stage
`.env`. If the variable name is unclear, inspect `.env` keys only and ask for
clarification.

## Documentation Expectations

When adding or changing architecture, update the relevant doc:

- `docs/architecture.md` — canonical control-plane design,
- `docs/runtime.md` — runtime/agent lifecycle internals,
- `docs/agent.md` — agent behavior,
- `docs/mcdr.md` — MCDR integration contract,
- `docs/lucy.md` / `docs/lucy-integration.md` — Lucy contract,
- `docs/checkpoints.md` / `docs/world-profile.md` — checkpoint and world rules,
- `docs/operations.md` — Operation coordination,
- `docs/storage.md` — storage backend rules,
- `docs/security.md` — security model,
- `docs/mvp.md` — MVP scope,
- `docs/status.md` — current implementation status (keep accurate),
- `docs/routemap.md` — phase roadmap (note: currently stale; refresh when
  touching roadmap content),
- `docs/cli-reference.md` — CLI surface (regenerate-aware when adding commands),
- `docs/workflow.md` — practical development workflow.

`README.md` and `README.zh.md` are public-facing; detailed design belongs in
`docs/`. If `AGENTS.md` becomes too large, move topic-specific guidance into
`docs/` and link from here.

## Current Product Direction

StratumMC is a Project/Room-centered collaborative Minecraft technical testing
platform — **not** a per-user unlimited sandbox cloud.

Default user flow:

```text
User joins Project
  → enters shared Room
  → collaborates with others
  → creates temporary Fork Session for risky experiments
  → saves useful result as Checkpoint
  → optionally promotes Checkpoint to a project milestone
```

## Before Finishing a Task

1. Run available tests (`go test -count=1 ./...`).
2. Run `gofmt` on changed Go files (`task fmt`).
3. Run `go vet ./...` (`task vet`).
4. Confirm new files fit existing architecture (see Repository Structure and
   Architectural Boundaries above).
5. Update docs if architecture changed.
6. Report using the standard structure below.

## Final Response Format

Every task completion response must use this structure:

1. **Verification**
   - `gofmt` status
   - `go test -count=1 ./...` status
   - `git diff --check` status
2. **Atomic commit summary**
   - `Commit 1: <prefix>: <summary>`
     - files changed
     - reason
   - additional commits in the same format when needed
3. **Behavior changes**
4. **Remaining TODOs**
5. **Suggested next atomic task**

See `docs/workflow.md` for the practical workflow this maps to.
