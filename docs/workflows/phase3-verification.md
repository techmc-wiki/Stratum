# Phase 3 Verification Checklist

Manual verification of World Management and Runtime Execution features.

## Prerequisites

```bash
# Build all binaries
go build ./cmd/stratum-agent/
go build ./cmd/stratum-controller/
go build ./cmd/stratum/

# Start agent and controller in separate terminals
./stratum-agent serve --listen 127.0.0.1:8787 --runtime-root ./runtime-root --token test-token
./stratum-controller serve --listen 127.0.0.1:8080 --data-dir ./data --agent-url http://127.0.0.1:8787 --agent-token test-token
```

---

## 1. Session Lifecycle with MCDR RuntimeProfile

Goal: Verify MCDR process starts, Minecraft boots, and readiness check passes.

```bash
# Setup
./stratum --data-dir ./data environments create --id env-fabric --name "Fabric 1.17" --minecraft-version 1.17.1 --loader fabric --server-core fabric --runtime-profile mcdr-fabric-1.17
./stratum --data-dir ./data projects create --id proj-1 --name "Test Project"
./stratum --data-dir ./data rooms create --id room-1 --project proj-1 --name "Test Room" --environment env-fabric

# Create and start session
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions create --id sess-1 --project proj-1 --room room-1
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions start --id sess-1 --actor alice --runtime-profile mcdr-fabric-1.17

# Verify running state
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions inspect --id sess-1
# Expected: State=running, RuntimeProfileID=mcdr-fabric-1.17

# Check logs
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions logs --id sess-1
# Expected: Minecraft console output, "Done (" readiness pattern

# Send command
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions send-command --id sess-1 --command "save-all"

# Graceful stop
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions stop --id sess-1 --actor alice
# Expected: State=stopped, no errors
```

---

## 2. Checkpoint Create — Metadata Only

Goal: Verify metadata checkpoint creation with runtime status snapshot.

```bash
# Create checkpoint while session is running
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions start --id sess-1 --actor alice --runtime-profile mcdr-fabric-1.17

./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 checkpoints create --id cp-meta --session sess-1 --actor alice --notes "metadata test"

# Inspect
./stratum --data-dir ./data checkpoints inspect --id cp-meta
# Expected: Runtime Status Snapshot: yes, Process State: running, PID set

# List
./stratum --data-dir ./data checkpoints list --session sess-1
# Expected: cp-meta appears in list
```

---

## 3. Checkpoint Create — Best Effort

Goal: Verify `save-all flush` command is sent and world snapshot is created.

```bash
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 checkpoints create --id cp-best --session sess-1 --actor alice --consistency-level best_effort --notes "best effort test"

./stratum --data-dir ./data checkpoints inspect --id cp-best
# Expected: WorldStateRef=agent-local://..., snapshotSizeBytes > 0, snapshotSHA256 not empty
```

---

## 4. Checkpoint Create — Command Quiesced

Goal: Verify `save-off` → `save-all flush` → snapshot → `save-on` sequence.

```bash
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 checkpoints create --id cp-quiesced --session sess-1 --actor alice --consistency-level command_quiesced --notes "quiesced test"

./stratum --data-dir ./data checkpoints inspect --id cp-quiesced
# Expected: WorldStateRef, consistency metadata present
```

---

## 5. Checkpoint Create — Stopped Level

Goal: Verify stop → snapshot → restart sequence.

```bash
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 checkpoints create --id cp-stopped --session sess-1 --actor alice --consistency-level stopped --notes "stopped test"

./stratum --data-dir ./data checkpoints inspect --id cp-stopped
# Expected: WorldStateRef present

# Verify session is running again after stopped-level checkpoint
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions inspect --id sess-1
# Expected: State=running (restarted after snapshot)
```

---

## 6. Checkpoint Restore

Goal: Verify world restore from checkpoint to target session.

```bash
# Stop the source session
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions stop --id sess-1 --actor alice

# Create a target session
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions create --id sess-2 --project proj-1 --room room-1
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions stop --id sess-2 --actor alice

# Restore checkpoint to target (target must be stopped)
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 checkpoints restore --checkpoint cp-quiesced --target-session sess-2 --actor alice

# Inspect restored checkpoint
./stratum --data-dir ./data checkpoints inspect --id <restored-cp-id>
# Expected: new checkpoint with restoredRef in WorldStateRef
```

---

## 7. Restore with Auto-Stop/Auto-Start

Goal: Verify full orchestration.

```bash
# Start a session
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions start --id sess-1 --actor alice --runtime-profile mcdr-fabric-1.17

# Create another session to restore into (needs to exist and have been started once to get RuntimeProfileID)
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions start --id sess-2 --actor alice --runtime-profile mcdr-fabric-1.17
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions stop --id sess-2 --actor alice

# Create a best-effort checkpoint on sess-1
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 checkpoints create --id cp-auto --session sess-1 --actor alice --consistency-level best_effort

# Restore with auto-stop + auto-start
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 checkpoints restore --checkpoint cp-auto --target-session sess-2 --actor alice --auto-stop --auto-start
# Expected: "Session sess-2 stopped before restore." → "World state restored: ..." → "Session sess-2 started after restore"
```

---

## 8. Pre-Operation Checkpoint — Session Restart

Goal: Verify pre-op checkpoint before session restart.

```bash
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions start --id sess-1 --actor alice --runtime-profile mcdr-fabric-1.17

# Restart with pre-op checkpoint
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 sessions restart --id sess-1 --actor alice --pre-op-checkpoint

# Verify checkpoint was created
./stratum --data-dir ./data checkpoints list --session sess-1
# Expected: a KindPreOperation checkpoint with Notes "Pre-operation checkpoint before session restart"
```

---

## 9. Pre-Operation Checkpoint — Artifact Apply

Goal: Verify pre-op checkpoint before artifact apply execution.

```bash
# Upload and stage an artifact (requires prior setup)
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 artifacts upload --id art-1 --path ./test-mod.jar --actor alice
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 artifacts approve --id art-1 --actor admin
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 artifacts stage --id stage-1 --artifact art-1 --session sess-1 --actor alice
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 artifacts materialize --staging-plan stage-1

# Create apply plan
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 artifacts apply plan --session sess-1 --staging-plan stage-1 --actor alice --target "mods/test-mod.jar"

# Execute with pre-op checkpoint
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 artifacts apply execute --plan <plan-id> --pre-op-checkpoint
# Expected: "Pre-operation checkpoint created for session sess-1." → "Apply execution result..."

# Verify checkpoint
./stratum --data-dir ./data checkpoints list --session sess-1
# Expected: KindPreOperation with Notes "Pre-operation checkpoint before artifact apply"
```

---

## 10. Checkpoint Diff

Goal: Verify world profile diff between checkpoint and session.

```bash
./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 checkpoints create --id cp-diff --session sess-1 --actor alice --capture-world-profile

./stratum --data-dir ./data --agent-url http://127.0.0.1:8787 checkpoints diff --checkpoint cp-diff --session sess-1
# Expected: World Profile Diff showing any differences
```

---

## Success Criteria

- [ ] MCDR process starts with real PID
- [ ] Minecraft readiness pattern "Done (" detected in logs
- [ ] stdin commands delivered (save-all, stop)
- [ ] Graceful stop via stdin ("!!MCDR stop")
- [ ] All 4 consistency levels work (metadata_only, best_effort, command_quiesced, stopped)
- [ ] World snapshot zip created with SHA-256
- [ ] Restore unzips to target session work directory
- [ ] Auto-stop/auto-start orchestration works
- [ ] Pre-op checkpoint created before restart
- [ ] Pre-op checkpoint created before artifact apply
- [ ] Checkpoint diff shows world profile changes
- [ ] Crash detection reports correct exit code
- [ ] Session state transitions correct throughout

---

## Test Data Cleanup

```bash
rm -rf ./data ./runtime-root
```
