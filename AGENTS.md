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
* MCDR-based Minecraft runtime control,
* Lucy-based non-intrusive environment/dependency management.

Prioritize correctness, reproducibility, safe boundaries, and clean architecture over rapid feature accumulation.

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

MCDR is the Minecraft JVM runtime bridge.

StratumMC may call MCDR to:

* start Minecraft server,
* stop Minecraft server,
* restart Minecraft server,
* send commands,
* collect logs,
* expose in-game commands.

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
9. MCDR/runtime supervision must be abstracted behind interfaces.
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

## Before Finishing a Task

Before reporting completion:

1. Run available tests.
2. Run `gofmt` on changed Go files.
3. Check that new files fit existing architecture.
4. Update docs if architecture changed.
5. Summarize:

   * what changed,
   * why it changed,
   * how to test it,
   * what remains TODO.
