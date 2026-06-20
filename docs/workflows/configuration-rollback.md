# Workflow: Configuration Rollback

This workflow demonstrates rolling back world configuration after accidental or problematic changes.

## Scenario

A team member accidentally modified `server.properties` in the shared lab session, changing critical settings. The team needs to restore known-good configuration.

**Actors:**
- Lab admin: `admin-1`
- Team member who made changes: `member-1`

**Problem:**
- Shared session `lab-main` has incorrect configuration
- Seed changed, breaking reproducibility
- Difficulty changed, affecting mob behavior
- View distance reduced, impacting tests

**Goal:**
- Restore last known-good configuration from checkpoint

---

## Prerequisites

```bash
DATA_DIR="./stratum-data"
AGENT_URL="http://127.0.0.1:8787"
SESSION_ID="lab-main"
PROJECT_ID="redstone-lab"
```

---

## Step 1: Identify Problem

**Admin notices configuration issues:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions runtime-status \
  --id $SESSION_ID
```

**Output shows unexpected values:**
```
Session: lab-main
State: running
...
Environment Manifest:
  ...
```

**Check recent checkpoints:**

```bash
stratum --data-dir $DATA_DIR \
  checkpoints list \
  --session $SESSION_ID
```

**Output:**
```
cp-stable-config    lab-main  2026-06-19T10:00:00Z  admin-1    command_quiesced
cp-daily-backup     lab-main  2026-06-18T09:00:00Z  admin-1    command_quiesced
cp-milestone-v1     lab-main  2026-06-15T15:30:00Z  admin-1    command_quiesced
```

**Inspect last stable checkpoint:**

```bash
stratum --data-dir $DATA_DIR \
  checkpoints inspect \
  --id cp-stable-config
```

**Output:**
```
Checkpoint ID:      cp-stable-config
...
World Profile Snapshot:
  Level Type:          flat
  Difficulty:          hard
  Seed:                12345
  Generate Structures: false
  View Distance:       10
  Captured From:       server.properties
...
```

**Known-good configuration identified.**

---

## Step 2: Stop Session

**Admin stops session for safe configuration restore:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop \
  --id $SESSION_ID \
  --actor admin-1
```

**Output:**
```
Session stopped: lab-main
```

**Important:** Stopping ensures:
- No JVM file locks preventing world replacement
- Clean configuration apply
- No in-progress operations interrupted

---

## Step 3: Create Pre-Rollback Checkpoint

**Capture current (broken) state before rollback:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints create \
  --id pre-rollback-broken \
  --session $SESSION_ID \
  --actor admin-1 \
  --consistency-level metadata_only \
  --capture-world-profile \
  --notes "Broken config before rollback (for debugging)"
```

**This allows:**
- Comparing broken vs working config
- Re-analyzing what went wrong
- Potential data recovery if needed

---

## Step 3.5: Preview Configuration Differences (Optional)

**Compare stable checkpoint with current broken configuration:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints diff \
  --checkpoint cp-stable-config \
  --session $SESSION_ID
```

**Output:**
```
World Profile Diff:

  Checkpoint: cp-stable-config
  Session:    lab-main

  level-seed:          "12345" -> "67890"
  difficulty:          "hard"  -> "normal"
```

**This shows:**
- Exactly what will change during restore
- Confirmation of problematic settings
- Verification before applying changes

---

## Step 4: Restore Known-Good Configuration

**Restore world and configuration from stable checkpoint:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints restore \
  --checkpoint cp-stable-config \
  --target-session $SESSION_ID \
  --actor admin-1 \
  --apply-world-profile \
  --notes "Rollback to stable configuration"
```

**Output:**
```
World state restored: checkpoint=cp-stable-config target=lab-main restoredRef=...
World profile applied to target session
```

**Restored configuration:**
- level-seed: `12345`
- level-type: `flat`
- difficulty: `hard`
- view-distance: `10`
- generate-structures: `false`

---

## Step 5: Verify Configuration

**Admin verifies server.properties (if agent provides file read):**

```bash
# (Future capability: read session files)
# For now, verify by starting and checking runtime status
```

**Start session:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions start \
  --id $SESSION_ID \
  --actor admin-1
```

**Check runtime status:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions runtime-status \
  --id $SESSION_ID
```

**Verify Minecraft server logs confirm correct seed and level type.**

---

## Step 6: Document Incident

**Create operations log or notes:**

```bash
# Manual documentation:
# - What was changed
# - Who changed it
# - When rollback occurred
# - Checkpoint used for rollback

# Future: operations history may capture this automatically
```

**Inspect operations (if available):**

```bash
stratum --data-dir $DATA_DIR \
  operations list \
  --session $SESSION_ID
```

---

## Step 7: Prevent Future Issues

**Establish checkpoint schedule:**

```bash
# Daily stable checkpoint
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints create \
  --id daily-$(date +%Y%m%d) \
  --session $SESSION_ID \
  --actor admin-1 \
  --consistency-level command_quiesced \
  --capture-world-profile \
  --notes "Daily stable checkpoint"
```

**Create checkpoint before configuration changes:**

```bash
# Before any server.properties edits
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints create \
  --id pre-config-change \
  --session $SESSION_ID \
  --actor member-1 \
  --capture-world-profile \
  --notes "Before changing view distance"

# Make changes...

# Verify changes using diff
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints diff \
  --checkpoint pre-config-change \
  --session $SESSION_ID

# If changes are incorrect, rollback
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop --id $SESSION_ID --actor member-1

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints restore \
  --checkpoint pre-config-change \
  --target-session $SESSION_ID \
  --actor member-1 \
  --apply-world-profile

# If changes are good, create post-change checkpoint
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints create \
  --id post-config-change \
  --session $SESSION_ID \
  --actor member-1 \
  --capture-world-profile
```

---

## Advanced: Partial Configuration Restore

**Restore only specific configuration fields, leaving others unchanged:**

```bash
# Restore only seed and level-type, keep current difficulty and view-distance
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop --id $SESSION_ID --actor member-1

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints restore \
  --checkpoint cp-stable-config \
  --target-session $SESSION_ID \
  --actor admin-1 \
  --apply-world-profile \
  --apply-world-profile-fields "seed,level-type"
```

**Output:**
```
World state restored: checkpoint=cp-stable-config target=lab-main restoredRef=...
World profile fields applied: [seed level-type]
```

**Valid field values:**
- `seed` — level-seed
- `level-type` — level-type
- `difficulty` — difficulty
- `view-distance` — view-distance
- `generate-structures` — generate-structures
- `spawn-radius` — spawn-protection
- `generator-settings` — generator-settings

**How it works:**
- Reads current `server.properties` from target session
- Appends specified fields from checkpoint's WorldProfileSnapshot
- Minecraft uses "last occurrence wins" for properties, so appended values override existing ones
- Fields not listed are left unchanged

---

## Comparison: Broken vs Working Config

**Use diff command to compare:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints diff \
  --checkpoint cp-stable-config \
  --session $SESSION_ID
```

**Output:**
```
World Profile Diff:

  Checkpoint: cp-stable-config
  Session:    lab-main

  level-seed:    "12345" -> "67890"
  difficulty:    "hard"  -> "normal"
  view-distance: 10      -> 8
```

**Manual inspection (alternative):**

```bash
stratum --data-dir $DATA_DIR \
  checkpoints inspect \
  --id pre-rollback-broken

stratum --data-dir $DATA_DIR \
  checkpoints inspect \
  --id cp-stable-config
```

---

## Emergency Rollback Script

**Automate rollback for quick recovery:**

```bash
#!/bin/bash
# rollback.sh - Emergency configuration rollback

DATA_DIR="./stratum-data"
AGENT_URL="http://127.0.0.1:8787"
SESSION_ID="lab-main"
STABLE_CHECKPOINT="cp-stable-config"
ACTOR="admin-1"

echo "Stopping session..."
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop --id $SESSION_ID --actor $ACTOR

echo "Creating pre-rollback checkpoint..."
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints create \
  --id "pre-rollback-$(date +%s)" \
  --session $SESSION_ID \
  --actor $ACTOR \
  --capture-world-profile \
  --notes "Emergency rollback backup"

echo "Restoring stable configuration..."
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints restore \
  --checkpoint $STABLE_CHECKPOINT \
  --target-session $SESSION_ID \
  --actor $ACTOR \
  --apply-world-profile \
  --notes "Emergency rollback"

echo "Starting session..."
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions start --id $SESSION_ID --actor $ACTOR

echo "Rollback complete. Verify configuration."
```

**Usage:**
```bash
chmod +x rollback.sh
./rollback.sh
```

---

## Result

- Shared lab session restored to known-good configuration
- World state optionally restored (if needed)
- Pre-rollback state captured for analysis
- Future incidents can be resolved quickly

---

## Best Practices

1. **Regular stable checkpoints**
   - Daily or weekly stable environment snapshots
   - Always capture world profile

2. **Pre-change checkpoints**
   - Before any server.properties edits
   - Before testing new configurations

3. **Document checkpoint purpose**
   - Use descriptive notes
   - Include what was changed or tested

4. **Verify after rollback**
   - Check runtime status
   - Verify Minecraft logs
   - Test critical functionality

5. **Keep multiple checkpoint versions**
   - Don't rely on single checkpoint
   - Maintain history (daily-20260619, daily-20260620, etc.)

---

## See Also

- [CLI Reference](../cli-reference.md) - Checkpoint commands
- [World Profile](../world-profile.md) - Configuration capture and restore
- [Checkpoints](../checkpoints.md) - Checkpoint concepts
- [Reproducible Testing](reproducible-testing.md) - Preventive approach
