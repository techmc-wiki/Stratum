# World Profile Capture and Restore

## Overview

WorldProfile captures Minecraft world generation and gameplay configuration as semantic metadata. It enables reproducible environment restoration and explicit configuration management for technical testing.

See [Workflows](workflows/) for end-to-end usage examples including reproducible testing and configuration rollback.

## Domain Model

### WorldProfile

A long-lived named world configuration template.

```go
type WorldProfile struct {
    ID               string
    Name             string
    Seed             string
    LevelType        string
    GeneratorSettings string
    GenerateStructures bool
    SpawnRadius      int
    Difficulty       string
    ViewDistance     int
}
```

### WorldProfileSnapshot

An immutable point-in-time capture of world configuration from a running session.

```go
type WorldProfileSnapshot struct {
    Seed               string
    LevelType          string
    GeneratorSettings  string
    GenerateStructures bool
    SpawnRadius        int
    Difficulty         string
    ViewDistance       int
    MinecraftVersion   string
    CapturedFrom       string
}
```

**Captured from:** `server.properties` during checkpoint creation.

## Capture Workflow

### 1. Checkpoint Creation with WorldProfile

```go
cp, err := checkpoint.Create(ctx, repo, service.CreateRequest{
    SessionID:           "s-1",
    ActorID:             "user-1",
    CaptureWorldProfile: true,
    AgentClient:         agent,
    // ...
})
```

**Process:**
1. Agent reads `sessions/{id}/server.properties`
2. Parse configuration fields:
   - `level-seed`
   - `level-type`
   - `generator-settings`
   - `generate-structures`
   - `spawn-protection`
   - `difficulty`
   - `view-distance`
3. Create `WorldProfileSnapshot`
4. Attach to checkpoint metadata

**Safety:** ReadSessionFile enforces path safety (no `..` traversal, relative paths only).

## Restore Workflow

### 1. Checkpoint Restore with WorldProfile Application

```go
restored, err := checkpoint.Restore(ctx, repo, service.RestoreRequest{
    CheckpointID:      "cp-1",
    TargetSessionID:   "s-target",
    ActorID:           "user-1",
    ApplyWorldProfile: true,
    AgentClient:       agent,
})
```

**Process:**
1. Restore world snapshot
2. Generate `server.properties` from checkpoint's `WorldProfileSnapshot`
3. Write to `sessions/{target}/server.properties`

**Generated properties:**
```properties
# Minecraft server properties
# Applied from checkpoint world profile snapshot
level-seed=12345
level-type=flat
generator-settings=...
generate-structures=false
spawn-protection=8
difficulty=hard
view-distance=10
```

**Safety:** Target session MUST be in `Stopped` state (same as world restore requirement).

## Field Mapping

### server.properties → WorldProfileSnapshot

| server.properties key | WorldProfileSnapshot field | Type | Notes |
|-----------------------|---------------------------|------|-------|
| `level-seed` | `Seed` | string | Empty if not set |
| `level-type` | `LevelType` | string | Default: "default" |
| `generator-settings` | `GeneratorSettings` | string | JSON for superflat/custom |
| `generate-structures` | `GenerateStructures` | bool | Default: true |
| `spawn-protection` | `SpawnRadius` | int | Default: 16 |
| `difficulty` | `Difficulty` | string | Default: "normal" |
| `view-distance` | `ViewDistance` | int | Optional |

### WorldProfileSnapshot → server.properties

Reverse mapping via `serverproperties.FromWorldProfileSnapshot()`.

All fields written with comment header:
```properties
# Minecraft server properties
# Applied from checkpoint world profile snapshot
```

## Agent Protocol

### ReadSessionFile

```go
ReadSessionFile(ctx context.Context, sessionID, relativePath string) ([]byte, error)
```

**HTTP endpoint:**
```
GET /v1/sessions/{sessionID}/files/{relativePath}
```

**Safety checks:**
- Rejects absolute paths
- Rejects `..` path traversal
- Only reads within session directory

### WriteSessionFile

```go
WriteSessionFile(ctx context.Context, sessionID, relativePath string, data []byte) error
```

**HTTP endpoint:**
```
PUT /v1/sessions/{sessionID}/files/{relativePath}
Content-Type: application/octet-stream
```

**Safety checks:**
- Same as ReadSessionFile
- Creates parent directories automatically
- Atomic write with 0644 permissions

## Use Cases

### 1. Reproducible Testing Environment

**Goal:** Ensure all testers use identical world configuration.

```bash
# Create checkpoint with world profile
stratum checkpoints create --session s-lab --capture-world-profile

# Restore to new session with same configuration
stratum checkpoints restore cp-123 --session s-test --apply-world-profile
```

**Result:** `s-test` inherits seed, level-type, difficulty, view-distance from `s-lab`.

### 2. Experiment Branching

**Goal:** Fork session with guaranteed identical base configuration.

```bash
# Fork creates checkpoint with world profile
stratum sessions fork s-main --name risky-test

# Fork session inherits world configuration automatically
```

### 3. Configuration Rollback

**Goal:** Restore known-good world settings after accidental modification.

```bash
# Restore checkpoint with world profile
stratum checkpoints restore cp-stable --session s-main --apply-world-profile
```

**Result:** `server.properties` reverts to checkpoint's captured state.

## Limitations

### Not Captured

WorldProfile does NOT capture:
- Live world data (chunks, entities, block states) — use world snapshot
- Jar mods or datapacks — use artifact references
- MCDR config — separate MCDR integration
- Carpet rules — future extension
- Player data or statistics
- Server performance settings (max-tick-time, etc.)

### Not Applied on Restore

When `ApplyWorldProfile=false` (default):
- World files restored
- `server.properties` unchanged

This allows manual configuration management.

### Partial Application

Currently no support for selective field application.

**Future:** Flag for field-level control:
```go
ApplyWorldProfileFields: []string{"seed", "level-type"}
```

## Testing

### Unit Tests

- `internal/agent/serverproperties/parser_test.go`
  - Parse all fields from server.properties
  - Generate server.properties from snapshot
- `internal/checkpoint/service/service_test.go`
  - Capture WorldProfile during create
  - Apply WorldProfile during restore

### E2E Tests

- `internal/agent/local/e2e_test.go`
  - `TestE2EReadSessionFileServerProperties`
  - `TestE2ECheckpointCaptureServerProperties`
  - `TestE2EWriteSessionFileServerProperties`

Full pipeline coverage: file read → parse → snapshot → generate → file write.

## Security Considerations

### Path Safety

- All file operations enforce relative-path-only constraints
- `..` path traversal explicitly rejected
- Absolute paths rejected
- Session directory boundaries enforced

### Session State Requirements

- Restore requires target session in `Stopped` state
- JVM file locks prevent safe configuration replacement while running
- Prevents data corruption from mid-runtime config changes

### Audit Trail

All checkpoint create/restore operations logged to audit events with:
- Actor ID
- Timestamp
- Session ID
- Checkpoint ID
- Metadata (capturedWorldProfile, appliedWorldProfile flags)

## Future Extensions

### 1. Additional server.properties Fields

- `max-build-height`
- `simulation-distance`
- `hardcore`
- `pvp`
- `max-players`
- `spawn-animals`
- `spawn-monsters`

### 2. Carpet Rule Capture

Capture Carpet mod rule configuration:
- `/carpet list` output parsing
- Carpet config file reading
- Rule application on restore

### 3. Datapack Configuration

Capture enabled datapacks:
- `world/datapacks/` directory listing
- Pack metadata extraction
- Artifact reference for reproducibility

### 4. Configuration Conflict Detection

Pre-restore checks:
- Compare source vs target world config
- Warn on seed/level-type mismatches
- Optional backup of existing server.properties

### 5. Selective Field Application

Per-field control:
```go
RestoreRequest{
    ApplyWorldProfile: true,
    WorldProfileFields: []string{"seed", "difficulty"},
}
```

Apply only specified fields, leave others unchanged.

## Implementation Notes

### serverproperties Package

Location: `internal/agent/serverproperties/`

**Parse:** Read server.properties → WorldConfig → WorldProfileSnapshot

**FromWorldProfileSnapshot:** WorldProfileSnapshot → server.properties text

**Design:** Pure functions, no I/O. File operations in agent layer.

### Checkpoint Service Integration

Location: `internal/checkpoint/service/`

**Create:** Optionally captures WorldProfile via `CaptureWorldProfile` flag.

**Restore:** Optionally applies WorldProfile via `ApplyWorldProfile` flag.

**Independence:** WorldProfile operations independent from world snapshot operations. Both can succeed/fail independently.

### Agent Responsibility

Agent owns file I/O:
- Session file path resolution
- Directory creation
- Atomic writes
- Safety validation

Controller/service layers never directly touch session filesystems.

## References

- `AGENTS.md` - Project rules and architecture boundaries
- `docs/architecture.md` - Overall system design
- `docs/checkpoints.md` - Checkpoint domain concepts
- `docs/cli-reference.md` - CLI commands including checkpoint restore with world profile
- `docs/agent.md` - Agent protocol and responsibilities
- `docs/security.md` - Security boundaries and validation
