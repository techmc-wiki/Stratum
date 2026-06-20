# Workflow: Reproducible Testing Environment

This workflow demonstrates creating a standardized testing environment that multiple team members can reproduce exactly.

## Scenario

A technical testing team needs to ensure all members test redstone contraptions on identical world configurations (seed, level type, difficulty, view distance).

**Team:**
- Lead researcher: `lead-1`
- Team members: `member-1`, `member-2`

**Goal:**
- Lead creates standard testing environment
- Team members restore identical environment to their sessions

---

## Prerequisites

```bash
# Set variables
DATA_DIR="./stratum-data"
AGENT_URL="http://127.0.0.1:8787"

# Ensure agent is running
# (agent process must be started separately)
```

---

## Step 1: Create Project and Room

**Lead creates project:**

```bash
stratum --data-dir $DATA_DIR \
  projects create \
  --id redstone-lab \
  --name "Redstone Testing Lab"
```

**Output:**
```
Project created: redstone-lab
```

**Lead creates testing room:**

```bash
stratum --data-dir $DATA_DIR \
  rooms create \
  --id flat-test \
  --project redstone-lab \
  --name "Flat Testing Room" \
  --environment env-1.17-fabric
```

**Output:**
```
Room created: flat-test
```

---

## Step 2: Lead Creates Standard Environment

**Create lead's session:**

```bash
stratum --data-dir $DATA_DIR \
  sessions create \
  --id lead-standard \
  --project redstone-lab \
  --room flat-test
```

**Start session and configure world:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions start \
  --id lead-standard \
  --actor lead-1
```

**Lead manually configures `server.properties`:**
- seed: `12345`
- level-type: `flat`
- difficulty: `hard`
- view-distance: `10`
- generate-structures: `false`

**Lead restarts session to apply configuration:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions restart \
  --id lead-standard \
  --actor lead-1
```

---

## Step 3: Capture Standard Environment Checkpoint

**Create checkpoint with world profile:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints create \
  --id standard-env-v1 \
  --session lead-standard \
  --actor lead-1 \
  --consistency-level command_quiesced \
  --capture-world-profile \
  --notes "Standard redstone testing environment v1"
```

**Output:**
```
Checkpoint created: standard-env-v1
```

**Verify checkpoint captured world profile:**

```bash
stratum --data-dir $DATA_DIR \
  checkpoints inspect \
  --id standard-env-v1
```

**Output:**
```
Checkpoint ID:      standard-env-v1
Project ID:         redstone-lab
Room ID:            flat-test
Session ID:         lead-standard
...
World Profile Snapshot:
  Level Type:          flat
  Difficulty:          hard
  Seed:                12345
  Generate Structures: false
  Spawn Radius:        16
  View Distance:       10
  Captured From:       server.properties
...
```

---

## Step 4: Team Members Restore Standard Environment

**Member 1 creates session:**

```bash
stratum --data-dir $DATA_DIR \
  sessions create \
  --id member1-test \
  --project redstone-lab \
  --room flat-test
```

**Member 1 starts and stops session (to reach stopped state):**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions start \
  --id member1-test \
  --actor member-1

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop \
  --id member1-test \
  --actor member-1
```

**Member 1 restores standard environment:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints restore \
  --checkpoint standard-env-v1 \
  --target-session member1-test \
  --actor member-1 \
  --apply-world-profile
```

**Output:**
```
World state restored: checkpoint=standard-env-v1 target=member1-test restoredRef=...
World profile applied to target session
```

**Member 1 starts session:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions start \
  --id member1-test \
  --actor member-1
```

**Member 2 repeats same process:**

```bash
stratum --data-dir $DATA_DIR \
  sessions create \
  --id member2-test \
  --project redstone-lab \
  --room flat-test

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions start --id member2-test --actor member-2

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop --id member2-test --actor member-2

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints restore \
  --checkpoint standard-env-v1 \
  --target-session member2-test \
  --actor member-2 \
  --apply-world-profile

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions start --id member2-test --actor member-2
```

---

## Step 5: Verify Environment Consistency

**All members should now have identical:**
- World seed: `12345`
- Level type: `flat`
- Difficulty: `hard`
- View distance: `10`
- Generate structures: `false`

**Verify by inspecting runtime status:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions runtime-status \
  --id member1-test
```

---

## Result

All team members now test on identical world configurations:
- Same seed ensures reproducible world generation
- Same difficulty ensures consistent mob behavior
- Same view distance ensures consistent chunk loading
- Flat world eliminates terrain variables

---

## Updating Standard Environment

When lead needs to update the standard:

```bash
# Lead makes changes to lead-standard session
# ...

# Create new checkpoint version
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints create \
  --id standard-env-v2 \
  --session lead-standard \
  --actor lead-1 \
  --consistency-level command_quiesced \
  --capture-world-profile \
  --notes "Standard environment v2 - updated spawn radius"

# Team members restore new version
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop --id member1-test --actor member-1

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints restore \
  --checkpoint standard-env-v2 \
  --target-session member1-test \
  --actor member-1 \
  --apply-world-profile
```

---

## See Also

- [CLI Reference](../cli-reference.md) - Complete command documentation
- [Checkpoints](../checkpoints.md) - Checkpoint concepts
- [World Profile](../world-profile.md) - World configuration capture details
