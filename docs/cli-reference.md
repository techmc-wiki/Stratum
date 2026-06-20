# CLI Reference

StratumMC is a CLI-first tool. All functionality is available through the `stratum` command-line interface.

See [Workflows](workflows/) for end-to-end usage examples.

## Global Flags

```bash
stratum [global flags] <command> [command flags]
```

### Required Global Flags

- `--data-dir <path>`: Data directory containing StratumMC repositories (required)

### Optional Global Flags

- `--agent-url <url>`: Agent HTTP endpoint (e.g., `http://127.0.0.1:8787`)
  - Required for commands that interact with running sessions or runtime state
  - Optional for metadata-only commands

## Command Categories

- [projects](#projects) - Manage projects
- [rooms](#rooms) - Manage rooms
- [sessions](#sessions) - Manage sessions
- [checkpoints](#checkpoints) - Manage checkpoints
- [artifacts](#artifacts) - Manage artifacts
- [environments](#environments) - Manage environments
- [operations](#operations) - View operation history
- [agents](#agents) - Manage agents
- [lucy](#lucy) - Lucy dependency management

---

## checkpoints

Manage semantic experiment snapshots.

### checkpoints create

Create a checkpoint from a session.

**Usage:**
```bash
stratum checkpoints create \
  --id <checkpoint-id> \
  --session <session-id> \
  --actor <actor-id> \
  [--notes <notes>] \
  [--consistency-level <level>] \
  [--capture-world-profile]
```

**Flags:**
- `--id` (required): Checkpoint ID
- `--session` (required): Source session ID
- `--actor` (required): Actor performing the operation
- `--notes` (optional): Checkpoint notes or description
- `--consistency-level` (optional): Consistency level (default: `metadata_only`)
  - `metadata_only`: Capture metadata and runtime status only. No world files saved.
  - `best_effort`: Send `save-all flush`, then capture world snapshot. Best-effort without stopping Minecraft writes.
  - `command_quiesced`: Send `save-off`, `save-all flush`, capture world snapshot, `save-on`. Stronger consistency than best_effort.
- `--capture-world-profile` (optional): Capture world configuration from `server.properties`

**Requires:** `--agent-url` when `--capture-world-profile` is used

**Example:**
```bash
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  checkpoints create \
  --id cp-experiment-1 \
  --session session-lab \
  --actor researcher-1 \
  --consistency-level command_quiesced \
  --capture-world-profile \
  --notes "Base state before redstone experiment"
```

**Output:**
```
Checkpoint created: cp-experiment-1
```

**See also:** [Checkpoints documentation](checkpoints.md), [World Profile documentation](world-profile.md)

---

### checkpoints diff

Compare checkpoint world profile with session current configuration.

**Usage:**
```bash
stratum checkpoints diff \
  --checkpoint <checkpoint-id> \
  --session <session-id>
```

**Flags:**
- `--checkpoint` (required): Checkpoint ID with world profile snapshot
- `--session` (required): Session ID to compare against

**Requires:** `--agent-url`

**Requirements:**
- Checkpoint must have world profile snapshot (captured with `--capture-world-profile`)
- Session must have `server.properties` file

**Example:**
```bash
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  checkpoints diff \
  --checkpoint cp-stable-config \
  --session session-main
```

**Output (when differences exist):**
```
World Profile Diff:

  Checkpoint: cp-stable-config
  Session:    session-main

  level-seed:          "12345" -> "67890"
  level-type:          "flat"  -> "default"
  difficulty:          "hard"  -> "normal"
  view-distance:       10      -> 8
  generate-structures: false   -> true
```

**Output (when no differences):**
```
World Profile Diff:

  Checkpoint: cp-stable-config
  Session:    session-main
```

**Use case:**
- Preview configuration changes before restore
- Verify session configuration matches checkpoint
- Identify configuration drift

**Implementation:**
- Reads checkpoint's `WorldProfileSnapshot` from controller store
- Calls `GetSessionRuntimeStatus` to obtain session's auto-populated `WorldProfile` from `work/server.properties`
- Compares field-by-field and outputs differences

**See also:** [Checkpoints documentation](checkpoints.md), [World Profile documentation](world-profile.md)

---

### checkpoints list

List checkpoints.

**Usage:**
```bash
stratum checkpoints list [--session <session-id>]
```

**Flags:**
- `--session` (optional): Filter by session ID

**Example:**
```bash
stratum --data-dir ./data checkpoints list --session session-lab
```

**Output:**
```
cp-experiment-1  session-lab  2026-06-20T10:30:00Z  researcher-1  command_quiesced
cp-milestone-1   session-lab  2026-06-19T15:00:00Z  lead-1        command_quiesced
```

---

### checkpoints inspect

Inspect checkpoint metadata.

**Usage:**
```bash
stratum checkpoints inspect --id <checkpoint-id>
```

**Flags:**
- `--id` (required): Checkpoint ID

**Example:**
```bash
stratum --data-dir ./data checkpoints inspect --id cp-experiment-1
```

**Output:**
```
Checkpoint ID:      cp-experiment-1
Project ID:         project-lab
Room ID:            room-main
Session ID:         session-lab
Environment ID:     env-1.17
Kind:               manual
Status:             complete
Consistency Level:  command_quiesced
Notes:              Base state before redstone experiment

World Profile Snapshot:
  Level Type:          flat
  Difficulty:          hard
  Seed:                12345
  Generate Structures: false
  Spawn Radius:        8
  Captured From:       server.properties

Created At:         2026-06-20T10:30:00Z
```

---

### checkpoints restore

Restore world state from a checkpoint to a target session.

**Usage:**
```bash
stratum checkpoints restore \
  --checkpoint <checkpoint-id> \
  --target-session <session-id> \
  --actor <actor-id> \
  [--world-dir <relative-dir>] \
  [--notes <notes>] \
  [--apply-world-profile] \
  [--apply-world-profile-fields <fields>]
```

**Flags:**
- `--checkpoint` (required): Source checkpoint ID
- `--target-session` (required): Target session ID
- `--actor` (required): Actor performing the restore
- `--world-dir` (optional): Relative world directory name (default: `world_restored`)
- `--notes` (optional): Notes for the restore operation
- `--apply-world-profile` (optional): Apply world profile (`server.properties` configuration) from checkpoint
- `--apply-world-profile-fields` (optional): Comma-separated fields to apply when using `--apply-world-profile`. Valid values: `seed`, `level-type`, `difficulty`, `view-distance`, `generate-structures`, `spawn-radius`, `generator-settings`. When omitted, all fields are applied.

**Partial Application Example:**
```bash
# Restore only seed and level-type, keep current difficulty and view-distance
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  checkpoints restore \
  --checkpoint cp-experiment-1 \
  --target-session session-test \
  --actor user-1 \
  --apply-world-profile \
  --apply-world-profile-fields "seed,level-type"
```
**Requires:** `--agent-url`

**Requirements:**
- Target session MUST be in `stopped` state
- Source checkpoint must have world snapshot
- When `--apply-world-profile` is used, checkpoint must have world profile snapshot

**Example:**
```bash
# Ensure target session is stopped
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions stop --id session-test --actor user-1

# Restore world and configuration
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  checkpoints restore \
  --checkpoint cp-experiment-1 \
  --target-session session-test \
  --actor user-1 \
  --apply-world-profile \
  --notes "Restore stable environment"
```

**Output:**
```
World state restored: checkpoint=cp-experiment-1 target=session-test restoredRef=...
World profile applied to target session
```

**World Profile Application:**

When `--apply-world-profile` is used, the following fields are written to target session's `server.properties`:
- `level-seed`
- `level-type`
- `generator-settings`
- `generate-structures`
- `spawn-protection`
- `difficulty`
- `view-distance`

**See also:** [World Profile documentation](world-profile.md)

---

## sessions

Manage Minecraft server sessions.

### sessions create

Create a new session.

**Usage:**
```bash
stratum sessions create \
  --id <session-id> \
  --project <project-id> \
  --room <room-id> \
  [--environment <environment-id>] \
  [--runtime-profile <profile-id>]
```

**Flags:**
- `--id` (required): Session ID
- `--project` (required): Project ID
- `--room` (required): Room ID
- `--environment` (optional): Environment ID (overrides room default)
- `--runtime-profile` (optional): Runtime profile ID

**Example:**
```bash
stratum --data-dir ./data \
  sessions create \
  --id session-test \
  --project project-lab \
  --room room-main
```

---

### sessions start

Start a session.

**Usage:**
```bash
stratum sessions start --id <session-id> --actor <actor-id>
```

**Flags:**
- `--id` (required): Session ID
- `--actor` (required): Actor performing the operation

**Requires:** `--agent-url`

**Example:**
```bash
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions start --id session-test --actor user-1
```

---

### sessions stop

Stop a running session.

**Usage:**
```bash
stratum sessions stop --id <session-id> --actor <actor-id>
```

**Flags:**
- `--id` (required): Session ID
- `--actor` (required): Actor performing the operation

**Requires:** `--agent-url`

**Example:**
```bash
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions stop --id session-test --actor user-1
```

---

### sessions list

List sessions.

**Usage:**
```bash
stratum sessions list
```

**Example:**
```bash
stratum --data-dir ./data sessions list
```

---

### sessions inspect

Inspect session metadata and runtime status.

**Usage:**
```bash
stratum sessions inspect --id <session-id>
```

**Flags:**
- `--id` (required): Session ID

**Example:**
```bash
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions inspect --id session-test
```

---

### sessions runtime-status

Inspect session runtime layout including WorldProfile.

**Usage:**
```bash
stratum sessions runtime-status --id <session-id>
```

**Flags:**
- `--id` (required): Session ID

**Requires:** `--agent-url`

**Returns:**
- Runtime directory structure (root, session, work, config, logs, etc.)
- Environment manifest status
- MCDR layout status
- Materialized and applied artifacts counts
- Process status (PID, runtime profile, running/crashed)
- **WorldProfile** — current server.properties values (seed, level-type, difficulty, view-distance, generate-structures, spawn-protection, generator-settings)

The Agent auto-populates WorldProfile by reading and parsing `work/server.properties`. This is the same source consumed by `checkpoints diff` and `checkpoints create --capture-world-profile`.

**Example:**
```bash
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions runtime-status --id session-test
```

**See also:** [CLI Reference](cli-reference.md), [World Profile documentation](world-profile.md)
---

## projects

Manage projects (long-term collaboration units).

### projects create

Create a project.

**Usage:**
```bash
stratum projects create --id <project-id> --name <name>
```

**Flags:**
- `--id` (required): Project ID
- `--name` (required): Project display name

**Example:**
```bash
stratum --data-dir ./data \
  projects create --id project-lab --name "Technical Testing Lab"
```

---

### projects list

List projects.

**Usage:**
```bash
stratum projects list
```

**Example:**
```bash
stratum --data-dir ./data projects list
```

---

## rooms

Manage rooms (shared workspaces within projects).

### rooms create

Create a room.

**Usage:**
```bash
stratum rooms create \
  --id <room-id> \
  --project <project-id> \
  --name <name> \
  --environment <environment-id>
```

**Flags:**
- `--id` (required): Room ID
- `--project` (required): Project ID
- `--name` (required): Room display name
- `--environment` (required): Default environment ID

**Example:**
```bash
stratum --data-dir ./data \
  rooms create \
  --id room-main \
  --project project-lab \
  --name "Main Testing Room" \
  --environment env-1.17
```

---

### rooms list

List rooms.

**Usage:**
```bash
stratum rooms list [--project <project-id>]
```

**Flags:**
- `--project` (optional): Filter by project ID

**Example:**
```bash
stratum --data-dir ./data rooms list --project project-lab
```

---

## Common Workflows

### Reproducible Testing Environment

Create a checkpoint with world profile, then restore to multiple test sessions:

```bash
# 1. Capture stable state
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  checkpoints create \
  --id stable-v1 \
  --session main-lab \
  --actor lead \
  --consistency-level command_quiesced \
  --capture-world-profile \
  --notes "Stable testing environment v1"

# 2. Create test session
stratum --data-dir ./data \
  sessions create \
  --id test-session-1 \
  --project project-lab \
  --room room-test

# 3. Start and stop to reach stopped state
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions start --id test-session-1 --actor tester-1

stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions stop --id test-session-1 --actor tester-1

# 4. Restore stable environment
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  checkpoints restore \
  --checkpoint stable-v1 \
  --target-session test-session-1 \
  --actor tester-1 \
  --apply-world-profile

# 5. Start testing
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions start --id test-session-1 --actor tester-1
```

---

### Experiment Branching

Fork a session for dangerous experiments:

```bash
# 1. Create checkpoint before risky work
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  checkpoints create \
  --id pre-experiment \
  --session shared-lab \
  --actor researcher \
  --capture-world-profile

# 2. Create fork session
stratum --data-dir ./data \
  sessions create \
  --id fork-risky-test \
  --project project-lab \
  --room room-temp

# 3. Restore to fork
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions start --id fork-risky-test --actor researcher

stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions stop --id fork-risky-test --actor researcher

stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  checkpoints restore \
  --checkpoint pre-experiment \
  --target-session fork-risky-test \
  --actor researcher \
  --apply-world-profile

# 4. Run dangerous experiment
stratum --data-dir ./data --agent-url http://127.0.0.1:8787 \
  sessions start --id fork-risky-test --actor researcher
```

---

## Exit Codes

- `0`: Success
- `1`: Operational error (validation, state transition, agent communication)
- `2`: Usage error (missing required flags, invalid arguments)

---

## See Also

- [Architecture documentation](architecture.md)
- [Checkpoints documentation](checkpoints.md)
- [World Profile documentation](world-profile.md)
- [Workflow documentation](workflow.md)
