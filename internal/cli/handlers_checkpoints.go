package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/checkpoint/consistency"
	checkpointsvc "github.com/stratummc/stratum/internal/checkpoint/service"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func createCheckpoint(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("checkpoints create", stderr)
	id := flags.String("id", "", "")
	sessionID := flags.String("session", "", "")
	actor := flags.String("actor", "", "")
	notes := flags.String("notes", "", "")
	consistencyLevelValue := flags.String("consistency-level", string(consistency.LevelMetadataOnly), "")
	captureWorldProfile := flags.Bool("capture-world-profile", false, "capture world profile from room")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *sessionID == "" || *actor == "" {
		fmt.Fprintln(stderr, "--id, --session, and --actor are required")
		return 2
	}
	consistencyLevel, err := consistency.Parse(*consistencyLevelValue)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --consistency-level: %v\n", err)
		return 2
	}
	if consistencyLevel != consistency.LevelMetadataOnly && consistencyLevel != consistency.LevelStopped && consistencyLevel != consistency.LevelBestEffort && consistencyLevel != consistency.LevelCommandQuiesced {
		fmt.Fprintf(stderr, "unsupported --consistency-level %q: only %q, %q, %q and %q are supported\n",
			consistencyLevel, consistency.LevelMetadataOnly, consistency.LevelStopped, consistency.LevelBestEffort, consistency.LevelCommandQuiesced)
		return 2
	}
	var snapshot *checkpoint.RuntimeStatusSnapshot
	lucyLockHash := ""
	if hasAgentURL {
		status, err := agentClient.GetSessionRuntimeStatus(ctx, *sessionID)
		if err == nil {
			value := checkpointRuntimeStatusSnapshot(status)
			snapshot = &value
			if status.EnvironmentManifest != nil {
				lucyLockHash = status.EnvironmentManifest.LucyLockHash
			}
		}
	}
	cp, err := checkpointsvc.Create(ctx, store, checkpointsvc.CreateRequest{
		ID:                    *id,
		SessionID:             *sessionID,
		ActorID:               *actor,
		Notes:                 *notes,
		ConsistencyLevel:      consistencyLevel,
		RuntimeStatusSnapshot: snapshot,
		LucyLockHash:          lucyLockHash,
		AgentClient:           agentClient,
		CaptureWorldProfile:   *captureWorldProfile,
	})
	if err != nil {
		fmt.Fprintf(stderr, "create checkpoint error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Checkpoint created: %s\n", cp.ID)
	return 0
}

func listCheckpoints(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("checkpoints list", stderr)
	sessionID := flags.String("session", "", "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	var values []checkpoint.Checkpoint
	var err error
	if *sessionID != "" {
		values, err = checkpointsvc.ListBySession(ctx, store, *sessionID)
	} else {
		values, err = checkpointsvc.List(ctx, store)
	}
	if err != nil {
		fmt.Fprintf(stderr, "list checkpoints error: %v\n", err)
		return 1
	}
	for _, cp := range values {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", cp.ID, cp.ProjectID, cp.SourceSessionID, cp.Status, cp.ConsistencyLevel, cp.Kind, cp.CreatedAt.Format(time.RFC3339))
	}
	return 0
}

func inspectCheckpoint(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("checkpoints inspect", stderr)
	id := flags.String("id", "", "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	cp, err := checkpointsvc.Get(ctx, store, *id)
	if err != nil {
		fmt.Fprintf(stderr, "get checkpoint error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "ID:                 %s\n", cp.ID)
	fmt.Fprintf(stdout, "Project ID:         %s\n", cp.ProjectID)
	fmt.Fprintf(stdout, "Room ID:            %s\n", cp.RoomID)
	fmt.Fprintf(stdout, "Source Session ID:  %s\n", cp.SourceSessionID)
	fmt.Fprintf(stdout, "Creator:            %s\n", cp.CreatorID)
	fmt.Fprintf(stdout, "Kind:               %s\n", cp.Kind)
	fmt.Fprintf(stdout, "Status:             %s\n", cp.Status)
	fmt.Fprintf(stdout, "Consistency Level:  %s\n", cp.ConsistencyLevel)
	fmt.Fprintf(stdout, "Environment ID:     %s\n", cp.EnvironmentID)
	if cp.RuntimeProfileID != "" {
		fmt.Fprintf(stdout, "Runtime Profile ID: %s\n", cp.RuntimeProfileID)
	}
	if cp.Notes != "" {
		fmt.Fprintf(stdout, "Notes:              %s\n", cp.Notes)
	}
	if cp.RuntimeStatusSnapshot == nil {
		fmt.Fprintln(stdout, "Runtime Status Snapshot: no")
	} else {
		snapshot := cp.RuntimeStatusSnapshot
		fmt.Fprintln(stdout, "Runtime Status Snapshot: yes")
		fmt.Fprintf(stdout, "  Captured At:                %s\n", snapshot.CapturedAt.Format(time.RFC3339))
		fmt.Fprintf(stdout, "  Overall Status:             %s\n", snapshot.OverallStatus)
		fmt.Fprintf(stdout, "  Environment Manifest:       %t\n", snapshot.EnvironmentManifestExists)
		fmt.Fprintf(stdout, "  Process State:              %s\n", snapshot.ProcessState)
		fmt.Fprintf(stdout, "  Materialized Artifacts:     %d\n", snapshot.MaterializedArtifactsCount)
		fmt.Fprintf(stdout, "  Applied Artifacts:          %d\n", snapshot.AppliedArtifactsCount)
		if len(snapshot.Issues) > 0 {
			fmt.Fprintf(stdout, "  Issues:                     %s\n", strings.Join(snapshot.Issues, ","))
		}
	}
	if cp.WorldProfileSnapshot != nil {
		ws := cp.WorldProfileSnapshot
		fmt.Fprintln(stdout, "\nWorld Profile Snapshot:")
		fmt.Fprintf(stdout, "  Level Type:          %s\n", ws.LevelType)
		fmt.Fprintf(stdout, "  Difficulty:          %s\n", ws.Difficulty)
		if ws.Seed != "" {
			fmt.Fprintf(stdout, "  Seed:                %s\n", ws.Seed)
		}
		fmt.Fprintf(stdout, "  Generate Structures: %v\n", ws.GenerateStructures)
		fmt.Fprintf(stdout, "  Spawn Radius:        %d\n", ws.SpawnRadius)
		if ws.GeneratorSettings != "" {
			fmt.Fprintf(stdout, "  Generator Settings:  %s\n", ws.GeneratorSettings)
		}
		if ws.CapturedFrom != "" {
			fmt.Fprintf(stdout, "  Captured From:       %s\n", ws.CapturedFrom)
		}
	}
	fmt.Fprintf(stdout, "Created At:         %s\n", cp.CreatedAt.Format(time.RFC3339))
	return 0
}

func restoreCheckpoint(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("checkpoints restore", stderr)
	checkpointID := flags.String("checkpoint", "", "")
	targetSessionID := flags.String("target-session", "", "")
	worldDir := flags.String("world-dir", "world_restored", "")
	actor := flags.String("actor", "", "")
	notes := flags.String("notes", "", "")
	applyWorldProfile := flags.Bool("apply-world-profile", false, "apply world profile from checkpoint to target session")
	applyWorldProfileFields := flags.String("apply-world-profile-fields", "", "comma-separated fields to apply (seed, level-type, difficulty, view-distance, generate-structures, spawn-radius, generator-settings)")
	autoStop := flags.Bool("auto-stop", false, "stop target session before restore")
	autoStart := flags.Bool("auto-start", false, "start target session after restore")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *checkpointID == "" || *targetSessionID == "" || *actor == "" {
		fmt.Fprintln(stderr, "--checkpoint, --target-session, and --actor are required")
		return 2
	}
	var fields []string
	if *applyWorldProfileFields != "" {
		fields = strings.Split(*applyWorldProfileFields, ",")
		for i, f := range fields {
			fields[i] = strings.TrimSpace(f)
		}
	}

	if *autoStop {
		if _, err := agentClient.StopSession(ctx, agent.SessionRequest{SessionID: *targetSessionID}); err != nil {
			return reportError(stderr, "checkpoint restore auto-stop", err)
		}
		fmt.Fprintf(stdout, "Session %s stopped before restore.\n", *targetSessionID)
	}

	cp, err := checkpointsvc.Restore(ctx, store, checkpointsvc.RestoreRequest{
		CheckpointID:            *checkpointID,
		TargetSessionID:         *targetSessionID,
		WorldDirRel:             *worldDir,
		ActorID:                 *actor,
		Notes:                   *notes,
		AgentClient:             agentClient,
		ApplyWorldProfile:       *applyWorldProfile,
		ApplyWorldProfileFields: fields,
	})
	if err != nil {
		return reportError(stderr, "checkpoint restore", err)
	}
	fmt.Fprintf(stdout, "World state restored: checkpoint=%s target=%s restoredRef=%s\n", cp.ID, *targetSessionID, cp.WorldStateRef)
	if *applyWorldProfile {
		if len(fields) > 0 {
			fmt.Fprintf(stdout, "World profile fields applied: %v\n", fields)
		} else {
			fmt.Fprintln(stdout, "World profile applied to target session")
		}
	}

	if *autoStart {
		sess, err := store.GetSession(ctx, *targetSessionID)
		if err != nil {
			return reportError(stderr, "restore auto-start get session", err)
		}
		if sess.RuntimeProfileID == "" {
			fmt.Fprintln(stderr, "session has no RuntimeProfileID; cannot auto-start after restore")
			return 2
		}
		if _, err := agentClient.StartSession(ctx, agent.SessionRequest{SessionID: *targetSessionID, RuntimeProfileID: sess.RuntimeProfileID}); err != nil {
			return reportError(stderr, "checkpoint restore auto-start", err)
		}
		fmt.Fprintf(stdout, "Session %s started after restore (profile=%s).\n", *targetSessionID, sess.RuntimeProfileID)
	}
	return 0
}

func diffCheckpoint(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("checkpoints diff", stderr)
	checkpointID := flags.String("checkpoint", "", "")
	sessionID := flags.String("session", "", "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *checkpointID == "" || *sessionID == "" {
		fmt.Fprintln(stderr, "--checkpoint and --session are required")
		return 2
	}
	cp, err := store.GetCheckpoint(ctx, *checkpointID)
	if err != nil {
		return reportError(stderr, "checkpoint diff", err)
	}
	if cp.WorldProfileSnapshot == nil {
		fmt.Fprintln(stderr, "checkpoint has no world profile snapshot")
		return 1
	}
	sess, err := store.GetSession(ctx, *sessionID)
	if err != nil {
		return reportError(stderr, "checkpoint diff", err)
	}
	status, err := agentClient.GetSessionRuntimeStatus(ctx, *sessionID)
	if err != nil {
		fmt.Fprintf(stderr, "failed to get session runtime status: %v\n", err)
		return 1
	}
	if status.WorldProfile == nil {
		fmt.Fprintln(stderr, "session has no world profile available")
		return 1
	}
	fmt.Fprintf(stdout, "World Profile Diff:\n\n")
	fmt.Fprintf(stdout, "  Checkpoint: %s\n", cp.ID)
	fmt.Fprintf(stdout, "  Session:    %s\n\n", sess.ID)
	cpw := cp.WorldProfileSnapshot
	sessw := status.WorldProfile
	if cpw.Seed != sessw.Seed {
		fmt.Fprintf(stdout, "  level-seed:          %q -> %q\n", cpw.Seed, sessw.Seed)
	}
	if cpw.LevelType != sessw.LevelType {
		fmt.Fprintf(stdout, "  level-type:          %q -> %q\n", cpw.LevelType, sessw.LevelType)
	}
	if cpw.Difficulty != sessw.Difficulty {
		fmt.Fprintf(stdout, "  difficulty:          %q -> %q\n", cpw.Difficulty, sessw.Difficulty)
	}
	if cpw.ViewDistance != sessw.ViewDistance {
		fmt.Fprintf(stdout, "  view-distance:       %d -> %d\n", cpw.ViewDistance, sessw.ViewDistance)
	}
	if cpw.GenerateStructures != sessw.GenerateStructures {
		fmt.Fprintf(stdout, "  generate-structures: %v -> %v\n", cpw.GenerateStructures, sessw.GenerateStructures)
	}
	if cpw.SpawnRadius != sessw.SpawnRadius {
		fmt.Fprintf(stdout, "  spawn-radius:        %d -> %d\n", cpw.SpawnRadius, sessw.SpawnRadius)
	}
	if cpw.GeneratorSettings != sessw.GeneratorSettings {
		fmt.Fprintf(stdout, "  generator-settings:  %q -> %q\n", cpw.GeneratorSettings, sessw.GeneratorSettings)
	}
	return 0
}

func checkpointRuntimeStatusSnapshot(status agent.SessionRuntimeStatus) checkpoint.RuntimeStatusSnapshot {
	snapshot := checkpoint.RuntimeStatusSnapshot{
		CapturedAt:        status.CheckedAt,
		SessionID:         status.SessionID,
		RuntimeRootExists: status.RuntimeRootExists,
		SessionRootExists: status.SessionRootExists,
		ProcessState:      "unknown",
		OverallStatus:     "ok",
	}
	if snapshot.CapturedAt.IsZero() {
		snapshot.CapturedAt = time.Now().UTC()
	}
	if !status.RuntimeRootExists {
		snapshot.Issues = append(snapshot.Issues, "runtime_root_missing")
	}
	if !status.SessionRootExists {
		snapshot.Issues = append(snapshot.Issues, "session_root_missing")
	}
	if status.EnvironmentManifest != nil {
		manifest := status.EnvironmentManifest
		snapshot.EnvironmentManifestExists = manifest.Exists
		snapshot.EnvironmentID = manifest.EnvironmentID
		snapshot.MinecraftVersion = manifest.MinecraftVersion
		snapshot.LoaderType = manifest.LoaderType
		snapshot.ServerCore = manifest.ServerCore
		snapshot.RuntimeProfileID = manifest.RuntimeProfileID
		if !manifest.Exists {
			snapshot.Issues = append(snapshot.Issues, "environment_manifest_missing")
		}
		if manifest.ErrorMessage != "" {
			snapshot.Issues = append(snapshot.Issues, "environment_manifest_error")
		}
	} else {
		snapshot.Issues = append(snapshot.Issues, "environment_manifest_unavailable")
	}
	if status.MCDRLayout != nil {
		snapshot.MCDRRootExists = status.MCDRLayout.MCDRRootExists
		snapshot.MCDRLayoutManifestExists = status.MCDRLayout.ManifestExists
	}
	if status.MaterializedArtifacts != nil {
		snapshot.MaterializedArtifactsCount = status.MaterializedArtifacts.Count
	}
	if status.AppliedArtifacts != nil {
		snapshot.AppliedArtifactsCount = status.AppliedArtifacts.Count
	}
	if status.ProcessStatus != nil {
		snapshot.ProcessState = status.ProcessStatus.Status
		snapshot.PID = status.ProcessStatus.PID
		if snapshot.RuntimeProfileID == "" {
			snapshot.RuntimeProfileID = status.ProcessStatus.RuntimeProfileID
		}
		if status.ProcessStatus.Crashed {
			snapshot.Issues = append(snapshot.Issues, "process_crashed")
		}
	} else {
		snapshot.Issues = append(snapshot.Issues, "process_status_unavailable")
	}
	if len(snapshot.Issues) > 0 {
		snapshot.OverallStatus = "issues"
	}
	return snapshot
}
