# AGENTS.md

## Project Overview

StratumMC is a high-end collaborative Minecraft technical testing control plane for invited advanced players, especially TMC/redstone/world-mechanics researchers.

GTMC will be the first deployment/community using StratumMC.

StratumMC is **not** a generic Minecraft hosting panel. It is a Project/Room-centered collaborative testing platform for:

* shared collaborative testing rooms,
* temporary fork sessions,
* semantic checkpoints,
* uploaded artifact management,
* resource-aware session scheduling,
* Agent-supervised Minecraft runtimes with optional future MCDR integration,
* Lucy-based non-intrusive environment/dependency management.

Prioritize correctness, reproducibility, safe boundaries, and clean architecture over rapid feature accumulation.

---

## Project Skills

If you don't have access to golang skills defined in @skills-lock.json , you should run `npx skills -y experimental_install` to fetch those skills, and tell your human partner to restart their agent client.

---

## Primary Language

Use **Go** as the primary implementation language.

Core components must be written in Go:

* controller
* agent
* CLI
* domain models
* services
* resource scheduler
* artifact manager
* checkpoint manager
* session manager
* API stubs
* runtime integration interfaces

Python is allowed only for:

* optional tooling,
* future MCDR plugins,
* migration scripts,
* world/NBT analysis helpers,
* admin utilities.

Do **not** implement the core controller or agent in Python unless explicitly requested.

---

## Core Domain Model

Use these concepts consistently.

### Project

A long-term collaboration unit.

A project contains:

* members,
* rooms,
* shared sessions,
* fork sessions,
* artifacts,
* checkpoints,
* permissions,
* audit history.

### Room

A collaborative workspace inside a project.

A room usually maps to one shared Minecraft session.

Examples:

* `1.12-main-lab`
* `1.17-flat-testing`
* `latest-normal-debug`

### Session

A runnable Minecraft server instance.

Session types:

* `shared`: long-lived collaborative room session.
* `fork`: temporary branch from a room or checkpoint for dangerous testing.
* `private`: short-lived personal sandbox.
* `review`: isolated session for testing uploaded artifacts.
* `archived`: stopped session with metadata only.

Default behavior should favor shared collaboration, not unlimited private instances.

### Checkpoint

A semantic experiment snapshot.

A checkpoint is not merely a zip backup. It should record:

* world state reference,
* environment reference,
* Lucy lock hash,
* artifacts/mods,
* server configs,
* Carpet rules,
* seed and generator settings,
* source room/session,
* creator,
* notes,
* operation history.

Dangerous operations should create a pre-operation checkpoint.

### Artifact

An uploaded file or reusable server asset.

Supported artifact types may include:

* `.jar` mod,
* datapack,
* MCDR plugin,
* config preset,
* Carpet rule preset,
* schematic/litematic,
* world archive.

StratumMC does **not** support user-provided URL mixin source compilation.

Players should upload `.jar` files or other explicit artifacts instead.

Artifacts must have:

* uploader,
* hash,
* compatibility metadata,
* approval status,
* usage records,
* creation time,
* optional review notes.

### Environment

A Minecraft runtime template.

An environment includes:

* Minecraft version,
* Java version,
* loader,
* server core,
* MCDR configuration,
* Carpet type,
* base mod set,
* Lucy manifest,
* Lucy lock file.

Initial long-term target versions:

* 1.12,
* 1.17,
* latest.

MVP should begin with:

* 1.17 Fabric + MCDR + Carpet.

### Resource Policy

StratumMC must assume limited server resources.

Resource policy should support:

* global max running sessions,
* per-project limits,
* per-user limits,
* session idle timeout,
* temporary session TTL,
* review session limits,
* queueing or denial reasons.

---

## Architectural Boundaries

### Runtime Ownership Rule

Stratum Agent owns the outer process and terminal lifecycle for every runtime.
It is responsible for process start, stop, restart, force termination, process
handles, terminal stdin/stdout/stderr, logs, local runtime status, PID and exit
code observation, crash detection, resource observation, and future sandboxing.

The Controller remains the source of truth for Project, Room, Session, and
Operation metadata, resource decisions, permissions, audit history, checkpoint
metadata, and artifact metadata. Agents report machine-local observations but
must not directly mutate Controller repositories or authoritative Session state.

MCDR is not the Stratum lifecycle source of truth. It may later be launched by
the Agent as a child process through a trusted RuntimeProfile. MCDR may manage
Minecraft console/plugin behavior inside that runtime, while Stratum Agent
supervises the outer process lifecycle. Future MCDR integration must be built
behind Agent runtime supervision; the Controller must not call MCDR directly as
its primary start/stop/restart manager.

### StratumMC Control Plane

StratumMC owns:

* Project Manager,
* Room Manager,
* Session Manager,
* Resource Scheduler,
* World Manager,
* Checkpoint Manager,
* Artifact Manager,
* Permission Manager,
* Audit Log,
* Storage Backend abstraction,
* public API / CLI / future Web UI.

### MCDR

MCDR is a possible future child runtime and server-side integration layer. It
may provide:

* send commands,
* expose in-game commands,
* bridge plugin and console behavior,
* manage Minecraft internally after the Agent starts its trusted runtime
  profile.

MCDR itself requires process supervision. Stratum Agent must be able to observe
its logs and exit status and stop or restart the outer runtime even when MCDR or
Minecraft is unresponsive.

Do not make Lucy responsible for any of these.

### Lucy

Lucy is strictly non-intrusive.

Lucy may be used for:

* dependency manifests,
* mod/plugin/server-core files,
* lock files,
* environment consistency checks,
* possible future backup integration.

Lucy must not be treated as:

* a JVM process manager,
* a live server controller,
* an MCDR replacement,
* a session scheduler.

### Uploaded Jars

Uploaded `.jar` files are potentially arbitrary code execution.

Never allow uploaded jars to affect:

* base worlds,
* other users’ sessions,
* unrelated projects,
* StratumMC controller files,
* checkpoint storage,
* host-level secrets.

Design for approval workflow and future sandboxing.

---

## MVP Scope

The first usable version should implement only:

* Go module and repository skeleton,
* project/room/session/checkpoint/artifact/resource-policy models,
* file-backed or in-memory repositories,
* one 1.17 Fabric + MCDR + Carpet environment template,
* shared room concept,
* fork session concept,
* session state transitions,
* checkpoint metadata create/list/rollback stubs,
* artifact metadata and hash calculation,
* resource policy checks,
* CLI or minimal API stubs,
* tests for core domain logic.

Do not implement these in MVP:

* full 1.12 support,
* full latest support,
* URL mixin compilation,
* automatic Minecraft world merge,
* complex chunk regeneration,
* RNG control,
* GTMC/StratumMC Debug Mod,
* full Web UI,
* production container orchestration,
* real Lucy integration beyond interface stubs,
* real MCDR process supervision beyond interface stubs.

---

## Go Repository Structure

Start close to this structure:

```text
/
  README.md
  AGENTS.md
  go.mod
  go.sum

  cmd/
    stratum/
      main.go
    stratum-controller/
      main.go
    stratum-agent/
      main.go

  internal/
    domain/
      project/
      room/
      session/
      checkpoint/
      artifact/
      environment/
      resourcepolicy/
      audit/

    service/
      projectsvc/
      roomsvc/
      sessionsvc/
      checkpointsvc/
      artifactsvc/
      schedulersvc/
      permissionsvc/

    repository/
      memory/
      filesystem/

    api/
      http/
      websocket/

    agent/
      process/
      mcdr/
      files/
      checkpoint/
      resource/

    integration/
      lucy/
      mcdr/

    config/
    errors/
    util/

  schemas/
    project.schema.json
    room.schema.json
    session.schema.json
    checkpoint.schema.json
    artifact.schema.json
    environment.schema.json
    resource-policy.schema.json
    audit-event.schema.json

  docs/
    architecture.md
    mvp.md
    security.md

  tools/
    python/
      README.md

  plugins/
    mcdr/
      README.md
```

This structure can evolve, but keep the controller/agent/domain/service/integration boundaries clear.

---

## Coding Guidelines

* Prefer clear module boundaries over clever abstractions.
* Keep domain logic independent from process execution.
* Do not hard-code deployment-specific filesystem paths in domain models.
* Use Go structs and typed constants/enums for domain models.
* Make session state transitions explicit and testable.
* Use structured errors with actionable messages.
* Avoid global mutable state unless clearly isolated for MVP.
* Write small functions with obvious responsibilities.
* Add TODO comments for external integration points.
* Do not silently swallow errors.
* Do not implement security-sensitive behavior as a mock without naming it clearly as a mock.
* Prefer the Go standard library unless a dependency clearly improves maintainability.
* Do not add large frameworks in the first skeleton unless necessary.

---

## Testing Expectations

Add tests whenever changing core logic.

Minimum test areas:

* resource policy decision behavior,
* session state transitions,
* checkpoint metadata creation,
* artifact hash metadata,
* permission checks,
* audit event creation.

Use standard Go testing.

Run:

```bash
go test ./...
```

Add linting/formatting later if the project standardizes on specific tools.

Always run:

```bash
gofmt
```

on changed Go files.

---

## Secrets / Commit Passphrase Rule:

If git commit requires an SSH/GPG passphrase, load it from local .env when available.
Never print, echo, log, commit, or expose the passphrase.
Never modify or stage .env.
If the variable name is unclear, inspect .env keys only and ask for clarification.

## Security Rules

Always preserve these rules:

1. Base worlds are immutable/read-only.
2. Shared rooms require stricter permissions than private/fork sessions.
3. Fork sessions are temporary and resource-controlled.
4. Dangerous operations create checkpoints first.
5. Uploaded jars must have metadata and hash records.
6. Unapproved artifacts must not be attached to shared sessions.
7. Review sessions should be isolated.
8. Lucy must not be used to control JVM runtime.
9. Stratum Agent owns runtime supervision; MCDR integration must remain an
   optional child RuntimeProfile behind Agent interfaces.
10. Never store secrets in committed files.
11. Do not allow uploaded artifacts to affect base worlds or unrelated sessions.
12. Do not implement URL mixin source compilation.

---

## Documentation Expectations

When adding or changing architecture, update relevant docs.

Important docs:

* `docs/architecture.md`
* `docs/mvp.md`
* `docs/security.md`

Public-facing README should explain what StratumMC is, but detailed design belongs in `docs/`.

If `AGENTS.md` becomes too large, move detailed topic-specific guidance into `docs/` and reference it from here.

---

## Current Product Direction

StratumMC should be designed as:

> A Project/Room-centered collaborative Minecraft technical testing platform.

Not as:

> A per-user unlimited Minecraft sandbox cloud.

The default flow should be:

```text
User joins Project
  -> enters shared Room
  -> collaborates with others
  -> creates temporary Fork Session only for risky experiments
  -> saves useful result as Checkpoint
  -> optionally promotes Checkpoint to project milestone
```

---

## Non-goals

Do not add these unless explicitly requested:

* automatic URL mixin source compilation,
* automatic merging of forked Minecraft worlds back into shared rooms,
* generic game panel features unrelated to StratumMC’s collaboration/testing model,
* production-grade billing/multi-tenant hosting,
* public registration,
* broad modpack hosting marketplace features,
* core controller/agent implementation in Python.

---

## Atomic Change / Commit Policy

Prefer small, reviewable changes over large multi-area rewrites. One Codex task
should normally correspond to one narrow implementation goal.

Do not mix unrelated changes, for example:

* runtime code + docs rewrite + CLI redesign + schema migration,
* feature implementation + broad refactor,
* bugfix + architecture rename.

If a task requires multiple logical steps, split the work into clear phases.
When git is available and committing is allowed, create atomic commits. Each
commit should compile and pass relevant tests when practical.

Use a consistent commit-message prefix:

* `docs:`
* `domain:`
* `storage:`
* `lifecycle:`
* `agent:`
* `runtime:`
* `cli:`
* `test:`
* `refactor:`
* `chore:`

If actual git commits cannot be created in the environment, summarize the work
as **proposed atomic commits** in the final response.

Additional rules:

* Do not touch unrelated files just to reformat or reword them.
* Do not rewrite docs wholesale unless the task is specifically documentation
  alignment.
* Do not update `AGENTS.md` during feature work unless project rules themselves
  need to change.
* Do not bundle architecture changes with implementation unless explicitly
  requested.
* Prefer focused tests for the changed module. Run the full suite before
  finishing when practical.
* Always summarize changed files grouped by logical commit.

The final response for future tasks must use this structure:

1. **Verification**
   * `gofmt` status
   * `go test -count=1 ./...` status
   * `git diff --check` status
2. **Atomic commit summary**
   * `Commit 1: <prefix>: <summary>`
     * files changed
     * reason
   * additional commits in the same format when needed
3. **Behavior changes**
4. **Remaining TODOs**
5. **Suggested next atomic task**

See `docs/workflow.md` for the practical development workflow.

---

## Before Finishing a Task

Before reporting completion:

1. Run available tests.
2. Run `gofmt` on changed Go files.
3. Check that new files fit existing architecture.
4. Update docs if architecture changed.
5. Report verification, proposed or actual atomic commits, behavior changes,
   remaining TODOs, and the suggested next atomic task using the format above.
