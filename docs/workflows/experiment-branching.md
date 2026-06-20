# Workflow: Experiment Branching

This workflow demonstrates forking a shared room into a temporary experiment session to test dangerous changes safely.

## Scenario

A researcher wants to test a potentially world-breaking redstone contraption without affecting the shared lab room.

**Actors:**
- Researcher: `researcher-1`

**Goal:**
- Fork shared room state to temporary session
- Test dangerous contraption in isolation
- Optionally merge successful results back

---

## Prerequisites

```bash
DATA_DIR="./stratum-data"
AGENT_URL="http://127.0.0.1:8787"

# Shared lab session already exists and is running
SHARED_SESSION="lab-main"
PROJECT_ID="redstone-lab"
```

---

## Step 1: Capture Shared Room State

**Create checkpoint before experiment:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints create \
  --id pre-experiment-baseline \
  --session $SHARED_SESSION \
  --actor researcher-1 \
  --consistency-level command_quiesced \
  --capture-world-profile \
  --notes "Baseline before testing chunk loader"
```

**Output:**
```
Checkpoint created: pre-experiment-baseline
```

---

## Step 2: Create Experiment Session

**Create temporary experiment session:**

```bash
stratum --data-dir $DATA_DIR \
  sessions create \
  --id experiment-chunk-loader \
  --project $PROJECT_ID \
  --room temp-experiments
```

**Start and stop to reach stopped state:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions start \
  --id experiment-chunk-loader \
  --actor researcher-1

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop \
  --id experiment-chunk-loader \
  --actor researcher-1
```

---

## Step 3: Restore Baseline to Experiment Session

**Restore shared room state with world profile:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints restore \
  --checkpoint pre-experiment-baseline \
  --target-session experiment-chunk-loader \
  --actor researcher-1 \
  --apply-world-profile \
  --notes "Fork for chunk loader experiment"
```

**Output:**
```
World state restored: checkpoint=pre-experiment-baseline target=experiment-chunk-loader restoredRef=...
World profile applied to target session
```

**Start experiment session:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions start \
  --id experiment-chunk-loader \
  --actor researcher-1
```

**Experiment session now has:**
- Identical world state to shared room
- Identical server.properties configuration
- Complete isolation from shared room

---

## Step 4: Test Dangerous Changes

**Researcher connects to experiment session and:**
- Builds chunk loader contraption
- Tests activation
- Observes behavior

**If contraption causes crash or corruption:**
- Experiment session is isolated
- Shared room remains unaffected
- Can delete experiment session and try again

---

## Step 5A: Experiment Failed - Discard

**If experiment fails, stop and delete:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop \
  --id experiment-chunk-loader \
  --actor researcher-1

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions archive \
  --id experiment-chunk-loader \
  --actor researcher-1

stratum --data-dir $DATA_DIR \
  sessions delete \
  --id experiment-chunk-loader \
  --actor researcher-1
```

**Shared room state unchanged.**

---

## Step 5B: Experiment Succeeded - Capture Results

**If experiment succeeds, capture successful state:**

```bash
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  checkpoints create \
  --id successful-chunk-loader \
  --session experiment-chunk-loader \
  --actor researcher-1 \
  --consistency-level command_quiesced \
  --capture-world-profile \
  --notes "Working chunk loader design"
```

**Share checkpoint with team:**

```bash
stratum --data-dir $DATA_DIR \
  checkpoints inspect \
  --id successful-chunk-loader
```

**Team members can inspect or restore to their own test sessions.**

---

## Step 6: Multiple Parallel Experiments

**Run multiple experiments simultaneously:**

```bash
# Experiment A: chunk loader
stratum --data-dir $DATA_DIR \
  sessions create \
  --id experiment-chunk-loader \
  --project $PROJECT_ID \
  --room temp-experiments

# Experiment B: flying machine
stratum --data-dir $DATA_DIR \
  sessions create \
  --id experiment-flying-machine \
  --project $PROJECT_ID \
  --room temp-experiments

# Both restore from same baseline
for session in experiment-chunk-loader experiment-flying-machine; do
  stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
    sessions start --id $session --actor researcher-1
  
  stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
    sessions stop --id $session --actor researcher-1
  
  stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
    checkpoints restore \
    --checkpoint pre-experiment-baseline \
    --target-session $session \
    --actor researcher-1 \
    --apply-world-profile
  
  stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
    sessions start --id $session --actor researcher-1
done
```

**Result:** Two isolated experiment sessions with identical baseline state.

---

## Resource Management

**Temporary experiment sessions consume resources:**
- Disk space for world files
- Memory when running
- CPU when active

**Clean up completed experiments:**

```bash
# List all experiment sessions
stratum --data-dir $DATA_DIR \
  sessions list | grep experiment-

# Archive and delete completed experiments
stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions stop --id experiment-chunk-loader --actor researcher-1

stratum --data-dir $DATA_DIR --agent-url $AGENT_URL \
  sessions archive --id experiment-chunk-loader --actor researcher-1

stratum --data-dir $DATA_DIR \
  sessions delete --id experiment-chunk-loader --actor researcher-1
```

---

## Best Practices

1. **Always create checkpoint before risky experiments**
   - Enables rollback if shared room is accidentally affected
   - Documents baseline state

2. **Use descriptive experiment session IDs**
   - `experiment-[feature]-[date]`
   - Easy to identify and clean up

3. **Capture successful experiments as checkpoints**
   - Enables sharing with team
   - Documents working designs

4. **Clean up experiment sessions regularly**
   - Archive when done
   - Delete to free resources

5. **Use --apply-world-profile for consistent environment**
   - Ensures experiment runs in same configuration as shared room
   - Eliminates configuration variables

---

## See Also

- [CLI Reference](../cli-reference.md) - Complete command documentation
- [World Profile](../world-profile.md) - World configuration capture and restore
- [Checkpoints](../checkpoints.md) - Checkpoint concepts and implementation
