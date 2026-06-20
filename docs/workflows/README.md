# Workflows

End-to-end workflow examples demonstrating common StratumMC usage patterns.

## Available Workflows

### [Reproducible Testing Environment](reproducible-testing.md)

Create a standardized testing environment that multiple team members can reproduce exactly.

**Use case:**
- Technical testing team needs identical world configurations
- Lead creates standard checkpoint with world profile
- Team members restore to their sessions

**Demonstrates:**
- Project and room creation
- Checkpoint creation with `--capture-world-profile`
- Checkpoint restore with `--apply-world-profile`
- Environment versioning

---

### [Experiment Branching](experiment-branching.md)

Fork a shared room into temporary experiment sessions to test dangerous changes safely.

**Use case:**
- Researcher wants to test potentially world-breaking contraption
- Fork shared state to isolated session
- Test without affecting shared room

**Demonstrates:**
- Checkpoint before risky operations
- Session forking via restore
- Isolated experiment sessions
- Resource cleanup
- Parallel experiments

---

### [Configuration Rollback](configuration-rollback.md)

Roll back world configuration after accidental or problematic changes.

**Use case:**
- Team member accidentally modified `server.properties`
- Critical settings changed (seed, difficulty, view distance)
- Need to restore known-good configuration

**Demonstrates:**
- Problem identification
- Pre-rollback snapshot
- Configuration restore with `--apply-world-profile`
- Verification workflow
- Incident documentation

---

## Common Patterns

### Checkpoint Before Dangerous Operations

```bash
stratum checkpoints create \
  --id pre-operation \
  --session $SESSION_ID \
  --actor $ACTOR \
  --consistency-level command_quiesced \
  --capture-world-profile
```

Always create checkpoint before:
- Configuration changes
- Risky experiments
- Major contraption tests
- Artifact application

---

### Stop-Restore-Start Pattern

```bash
# Stop session
stratum sessions stop --id $TARGET --actor $ACTOR

# Restore checkpoint
stratum checkpoints restore \
  --checkpoint $CHECKPOINT_ID \
  --target-session $TARGET \
  --actor $ACTOR \
  --apply-world-profile

# Start session
stratum sessions start --id $TARGET --actor $ACTOR
```

Required for safe world and configuration restoration.

---

### Environment Consistency Check

```bash
# Inspect checkpoint configuration
stratum checkpoints inspect --id $CHECKPOINT_ID

# Verify session configuration after restore
stratum sessions runtime-status --id $SESSION_ID
```

Ensure world profile applied correctly.

---

## CLI Reference

See [CLI Reference](../cli-reference.md) for complete command documentation.

## Architecture Documentation

- [Architecture Overview](../architecture.md) - System design principles
- [Checkpoints](../checkpoints.md) - Checkpoint concepts and implementation
- [World Profile](../world-profile.md) - World configuration capture and restore
- [Sessions](../sessions.md) - Session lifecycle and management
