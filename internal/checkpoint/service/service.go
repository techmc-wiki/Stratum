package service

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/serverproperties"
	"github.com/stratummc/stratum/internal/audit"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/checkpoint/consistency"
	"github.com/stratummc/stratum/internal/idgen"
	"github.com/stratummc/stratum/internal/room"
	"github.com/stratummc/stratum/internal/session"
)

type CreateRequest struct {
	ID                    string
	SessionID             string
	ActorID               string
	Notes                 string
	ConsistencyLevel      consistency.Level
	ConsistencyMetadata   map[string]string
	RuntimeStatusSnapshot *checkpoint.RuntimeStatusSnapshot
	LucyLockHash          string
	AgentClient           agent.AgentClient
	CaptureWorldProfile   bool
}

type SessionReader interface {
	GetSession(ctx context.Context, id string) (session.Session, error)
}

type RoomReader interface {
	GetRoom(ctx context.Context, id string) (room.Room, error)
}

type Repository interface {
	SessionReader
	RoomReader
	CreateCheckpoint(ctx context.Context, cp checkpoint.Checkpoint) error
	UpdateCheckpoint(ctx context.Context, cp checkpoint.Checkpoint) error
	GetCheckpoint(ctx context.Context, id string) (checkpoint.Checkpoint, error)
	ListCheckpoints(ctx context.Context) ([]checkpoint.Checkpoint, error)
	ListCheckpointsBySession(ctx context.Context, sessionID string) ([]checkpoint.Checkpoint, error)
	AppendAuditEvent(ctx context.Context, event audit.Event) error
}

// Create creates a checkpoint. Checkpoint creation is safe during session runtime because:
// - MetadataOnly: no world files are accessed
// - CommandQuiesced: uses save-off + save-all flush to ensure world files are read-only before snapshot
// World files may be held open by JVM but are safe to read (copy) while the server is running.
func Create(ctx context.Context, repo Repository, req CreateRequest) (checkpoint.Checkpoint, error) {
	if req.ActorID == "" {
		return checkpoint.Checkpoint{}, fmt.Errorf("actor required")
	}
	sess, err := repo.GetSession(ctx, req.SessionID)
	if err != nil {
		return checkpoint.Checkpoint{}, err
	}
	if req.RuntimeStatusSnapshot != nil && req.RuntimeStatusSnapshot.SessionID != "" && req.RuntimeStatusSnapshot.SessionID != sess.ID {
		return checkpoint.Checkpoint{}, fmt.Errorf("runtime status snapshot session %q does not match checkpoint session %q", req.RuntimeStatusSnapshot.SessionID, sess.ID)
	}
	consistencyLevel := req.ConsistencyLevel
	if consistencyLevel == "" {
		consistencyLevel = consistency.LevelMetadataOnly
	}
	switch consistencyLevel {
	case consistency.LevelMetadataOnly:
	case consistency.LevelCommandQuiesced:
		if err := validateCommandQuiesced(ctx, req, sess); err != nil {
			return checkpoint.Checkpoint{}, err
		}
		return createCommandQuiesced(ctx, repo, req, sess)
	default:
		return checkpoint.Checkpoint{}, fmt.Errorf("checkpoint consistency level %q requires checkpoint orchestration; only %q and %q are supported", consistencyLevel, consistency.LevelMetadataOnly, consistency.LevelCommandQuiesced)
	}
	cp, err := checkpoint.New(buildCheckpointParams(ctx, repo, req, sess, consistencyLevel))
	if err != nil {
		return checkpoint.Checkpoint{}, err
	}
	if err := repo.CreateCheckpoint(ctx, cp); err != nil {
		return checkpoint.Checkpoint{}, err
	}
	if err := repo.AppendAuditEvent(ctx, buildAuditEvent(req, cp)); err != nil {
		return cp, fmt.Errorf("checkpoint created but audit append failed: %w", err)
	}
	return cp, nil
}

func validateCommandQuiesced(ctx context.Context, req CreateRequest, sess session.Session) error {
	if req.AgentClient == nil {
		return fmt.Errorf("checkpoint consistency level %q requires an agent client", req.ConsistencyLevel)
	}
	if strings.TrimSpace(sess.RuntimeProfileID) == "" {
		if sess.EnvironmentID == "" {
			return fmt.Errorf("session %q does not have an environment or runtime profile", sess.ID)
		}
		// TODO: if Environment stores a RuntimeProfileID field, use it to determine stdin-command capability.
		// Currently the signal is EnvironmentID presence; we fall back to agent capabilities.
	}
	info, err := req.AgentClient.Info(ctx)
	if err != nil {
		return fmt.Errorf("check agent info: %w", err)
	}
	hasSendCommand := false
	for _, cap := range info.Capabilities {
		if cap == string(agent.OperationSendCommand) {
			hasSendCommand = true
			break
		}
	}
	if !hasSendCommand {
		return fmt.Errorf("agent %q does not support send-command; command_quiesced requires send-command capability", info.ID)
	}
	return nil
}

func createCommandQuiesced(ctx context.Context, repo Repository, req CreateRequest, sess session.Session) (checkpoint.Checkpoint, error) {
	consistencyMetadata := mergeMetadata(req.ConsistencyMetadata)

	saveOnRequired := true
	defer func() {
		if !saveOnRequired {
			return
		}
		if _, err := req.AgentClient.SendCommand(ctx, req.SessionID, "save-on"); err != nil {
			consistencyMetadata["saveOnError"] = err.Error()
		}
	}()

	if _, err := req.AgentClient.SendCommand(ctx, req.SessionID, "save-off"); err != nil {
		return checkpoint.Checkpoint{}, fmt.Errorf("save-off command failed: %w", err)
	}
	if _, err := req.AgentClient.SendCommand(ctx, req.SessionID, "save-all flush"); err != nil {
		return checkpoint.Checkpoint{}, fmt.Errorf("save-all flush command failed: %w", err)
	}

	snapResult, snapErr := req.AgentClient.CreateWorldSnapshot(ctx, agent.WorldCheckpointRequest{SessionID: req.SessionID})
	if snapErr != nil {
		return checkpoint.Checkpoint{}, fmt.Errorf("world snapshot failed: %w", snapErr)
	}

	consistencyMetadata["worldSnapshot"] = "true"
	consistencyMetadata["snapshotSizeBytes"] = fmt.Sprintf("%d", snapResult.SizeBytes)
	consistencyMetadata["snapshotSHA256"] = snapResult.SHA256

	params := buildCheckpointParams(ctx, repo, req, sess, consistency.LevelCommandQuiesced)
	params.ConsistencyMetadata = consistencyMetadata
	params.WorldStateRef = snapResult.SnapshotRef
	cp, err := checkpoint.New(params)
	if err != nil {
		return checkpoint.Checkpoint{}, err
	}
	if err := repo.CreateCheckpoint(ctx, cp); err != nil {
		return checkpoint.Checkpoint{}, err
	}

	saveOnRequired = false
	saveOnErr := saveOnWithError(ctx, req.AgentClient, req.SessionID)
	if saveOnErr != nil {
		consistencyMetadata["saveOnError"] = saveOnErr.Error()
		cp.ConsistencyMetadata = consistencyMetadata
		updateErr := repo.UpdateCheckpoint(ctx, cp)
		event := buildAuditEvent(req, cp)
		event.Metadata["worldSnapshot"] = "true"
		event.Metadata["commandQuiesced"] = "true"
		event.Metadata["snapshotSizeBytes"] = consistencyMetadata["snapshotSizeBytes"]
		event.Metadata["snapshotSHA256"] = consistencyMetadata["snapshotSHA256"]
		event.Metadata["saveOnError"] = saveOnErr.Error()
		if updateErr != nil {
			event.Metadata["updateCheckpointError"] = updateErr.Error()
		}
		auditErr := repo.AppendAuditEvent(ctx, event)
		if updateErr != nil {
			errMsg := fmt.Sprintf("save-on failed (%v) and update checkpoint failed (%v)", saveOnErr, updateErr)
			if auditErr != nil {
				errMsg = fmt.Sprintf("%s; audit append failed: %v", errMsg, auditErr)
			}
			return cp, fmt.Errorf("%s", errMsg)
		}
		if auditErr != nil {
			return cp, fmt.Errorf("save-on failed (%v) but persisted; audit append failed: %w", saveOnErr, auditErr)
		}
		return cp, nil
	}

	event := buildAuditEvent(req, cp)
	event.Metadata["worldSnapshot"] = "true"
	event.Metadata["commandQuiesced"] = "true"
	event.Metadata["snapshotSizeBytes"] = consistencyMetadata["snapshotSizeBytes"]
	event.Metadata["snapshotSHA256"] = consistencyMetadata["snapshotSHA256"]
	if err := repo.AppendAuditEvent(ctx, event); err != nil {
		return cp, fmt.Errorf("checkpoint created but audit append failed: %w", err)
	}
	return cp, nil
}

func saveOnWithError(ctx context.Context, agentClient agent.AgentClient, sessionID string) error {
	_, err := agentClient.SendCommand(ctx, sessionID, "save-on")
	return err
}

func buildCheckpointParams(ctx context.Context, repo Repository, req CreateRequest, sess session.Session, level consistency.Level) checkpoint.CreateParams {
	var worldSnapshot *checkpoint.WorldProfileSnapshot
	if req.CaptureWorldProfile {
		if req.AgentClient != nil {
			if data, err := req.AgentClient.ReadSessionFile(ctx, sess.ID, "server.properties"); err == nil {
				if cfg, err := serverproperties.Parse(bytes.NewReader(data)); err == nil {
					mcVersion := ""
					if envManifest, err := req.AgentClient.GetSessionRuntimeStatus(ctx, sess.ID); err == nil {
						if envManifest.EnvironmentManifest != nil {
							mcVersion = envManifest.EnvironmentManifest.MinecraftVersion
						}
					}
					worldSnapshot = serverproperties.ToWorldProfileSnapshot(cfg, mcVersion)
				}
			}
		}
		if worldSnapshot == nil && sess.RoomID != "" {
			if rm, err := repo.GetRoom(ctx, sess.RoomID); err == nil && rm.DefaultWorldProfile != nil {
				wp := rm.DefaultWorldProfile
				worldSnapshot = &checkpoint.WorldProfileSnapshot{
					Seed:               wp.Seed,
					LevelType:          string(wp.LevelType),
					GeneratorSettings:  wp.GeneratorSettings,
					GenerateStructures: wp.GenerateStructures,
					SpawnRadius:        wp.SpawnRadius,
					Difficulty:         string(wp.Difficulty),
					MinecraftVersion:   wp.MinecraftVersion,
					SourceProfileID:    wp.ID,
					CapturedFrom:       "room",
				}
			}
		}
	}

	return checkpoint.CreateParams{
		ID:                    req.ID,
		ProjectID:             sess.ProjectID,
		RoomID:                sess.RoomID,
		SourceSessionID:       sess.ID,
		CreatorID:             req.ActorID,
		Kind:                  checkpoint.KindManual,
		Status:                checkpoint.StatusMetadataOnly,
		ConsistencyLevel:      level,
		ConsistencyMetadata:   req.ConsistencyMetadata,
		EnvironmentID:         sess.EnvironmentID,
		RuntimeProfileID:      sess.RuntimeProfileID,
		LucyLockHash:          req.LucyLockHash,
		RuntimeStatusSnapshot: prepareRuntimeStatusSnapshot(req.RuntimeStatusSnapshot, sess),
		WorldProfileSnapshot:  worldSnapshot,
		Notes:                 req.Notes,
	}
}

func buildAuditEvent(req CreateRequest, cp checkpoint.Checkpoint) audit.Event {
	eventID, _ := idgen.NewID("audit")
	event, _ := audit.NewEvent(eventID, req.ActorID, "checkpoint.created", "checkpoint", cp.ID, time.Now().UTC())
	event.Metadata = map[string]string{
		"checkpointId":                  cp.ID,
		"projectId":                     cp.ProjectID,
		"roomId":                        cp.RoomID,
		"sessionId":                     cp.SourceSessionID,
		"environmentId":                 cp.EnvironmentID,
		"status":                        string(cp.Status),
		"consistencyLevel":              string(cp.ConsistencyLevel),
		"actor":                         req.ActorID,
		"runtimeStatusSnapshotCaptured": fmt.Sprintf("%t", cp.RuntimeStatusSnapshot != nil),
	}
	if cp.RuntimeProfileID != "" {
		event.Metadata["runtimeProfileId"] = cp.RuntimeProfileID
	}
	if cp.RuntimeStatusSnapshot != nil {
		event.Metadata["runtimeStatusOverallStatus"] = cp.RuntimeStatusSnapshot.OverallStatus
		event.Metadata["processState"] = cp.RuntimeStatusSnapshot.ProcessState
	}
	return event
}

func mergeMetadata(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func prepareRuntimeStatusSnapshot(snapshot *checkpoint.RuntimeStatusSnapshot, sess session.Session) *checkpoint.RuntimeStatusSnapshot {
	if snapshot == nil {
		return nil
	}
	result := *snapshot
	result.Issues = append([]string(nil), snapshot.Issues...)
	if result.SessionID == "" {
		result.SessionID = sess.ID
	}
	if result.RuntimeProfileID == "" {
		result.RuntimeProfileID = sess.RuntimeProfileID
	}
	return &result
}

func Get(ctx context.Context, repo Repository, id string) (checkpoint.Checkpoint, error) {
	return repo.GetCheckpoint(ctx, id)
}

func List(ctx context.Context, repo Repository) ([]checkpoint.Checkpoint, error) {
	return repo.ListCheckpoints(ctx)
}

func ListBySession(ctx context.Context, repo Repository, sessionID string) ([]checkpoint.Checkpoint, error) {
	return repo.ListCheckpointsBySession(ctx, sessionID)
}

type RestoreRequest struct {
	CheckpointID      string
	TargetSessionID   string
	WorldDirRel       string
	ActorID           string
	Notes             string
	AgentClient       agent.AgentClient
	ApplyWorldProfile bool
}

// Restore restores a checkpoint's world state to a target session.
// SAFETY: Target session MUST be in Stopped state because:
// - JVM locks jar/world files while running (cannot replace/delete)
// - Overwriting open region files causes data corruption
// - Windows file locks will cause restore to fail
// - Unix allows unsafe overwrites but data is corrupted
func Restore(ctx context.Context, repo Repository, req RestoreRequest) (checkpoint.Checkpoint, error) {
	if strings.TrimSpace(req.ActorID) == "" {
		return checkpoint.Checkpoint{}, fmt.Errorf("actor required")
	}
	if req.AgentClient == nil {
		return checkpoint.Checkpoint{}, fmt.Errorf("agent client required for restore")
	}
	sourceCP, err := repo.GetCheckpoint(ctx, req.CheckpointID)
	if err != nil {
		return checkpoint.Checkpoint{}, fmt.Errorf("get source checkpoint: %w", err)
	}
	if sourceCP.WorldStateRef == "" {
		return checkpoint.Checkpoint{}, fmt.Errorf("checkpoint %q has no world state", sourceCP.ID)
	}
	targetSession, err := repo.GetSession(ctx, req.TargetSessionID)
	if err != nil {
		return checkpoint.Checkpoint{}, fmt.Errorf("get target session: %w", err)
	}
	if targetSession.ProjectID != sourceCP.ProjectID {
		return checkpoint.Checkpoint{}, fmt.Errorf("target session project %q does not match source checkpoint project %q", targetSession.ProjectID, sourceCP.ProjectID)
	}
	if targetSession.State != session.StateStopped {
		return checkpoint.Checkpoint{}, fmt.Errorf("target session must be stopped before restore, current state: %s (JVM file locks prevent safe world replacement while running)", targetSession.State)
	}
	worldDirRel := strings.TrimSpace(req.WorldDirRel)
	if worldDirRel == "" {
		worldDirRel = "world_restored"
	}
	agentResult, err := req.AgentClient.RestoreWorldSnapshot(ctx, agent.WorldCheckpointRestoreRequest{
		SessionID:   req.TargetSessionID,
		SnapshotRef: sourceCP.WorldStateRef,
		WorldDirRel: worldDirRel,
	})
	if err != nil {
		return checkpoint.Checkpoint{}, fmt.Errorf("restore world snapshot: %w", err)
	}
	if req.ApplyWorldProfile && sourceCP.WorldProfileSnapshot != nil {
		propsContent := serverproperties.FromWorldProfileSnapshot(sourceCP.WorldProfileSnapshot)
		if err := req.AgentClient.WriteSessionFile(ctx, req.TargetSessionID, "server.properties", []byte(propsContent)); err != nil {
			return checkpoint.Checkpoint{}, fmt.Errorf("write server.properties: %w", err)
		}
	}
	checkpointID, idErr := idgen.NewID("cp")
	if idErr != nil {
		return checkpoint.Checkpoint{}, fmt.Errorf("generate checkpoint id: %w", idErr)
	}
	notes := strings.TrimSpace(req.Notes)
	if notes == "" {
		notes = "Restored from checkpoint " + sourceCP.ID
	}
	metadata := map[string]string{
		"restoredFromCheckpoint": sourceCP.ID,
		"restoredEntryCount":     fmt.Sprintf("%d", agentResult.EntryCount),
		"restoredSizeBytes":      fmt.Sprintf("%d", agentResult.SizeBytes),
		"worldDirRel":            worldDirRel,
	}
	params := checkpoint.CreateParams{
		ID:               checkpointID,
		ProjectID:        targetSession.ProjectID,
		RoomID:           targetSession.RoomID,
		SourceSessionID:  req.TargetSessionID,
		CreatorID:        req.ActorID,
		Kind:             checkpoint.KindManual,
		Status:           checkpoint.StatusMetadataOnly,
		ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID:    targetSession.EnvironmentID,
		RuntimeProfileID: targetSession.RuntimeProfileID,
		WorldStateRef:    agentResult.RestoredRef,
		LucyLockHash:     sourceCP.LucyLockHash,
		Notes:            notes,
		Metadata:         metadata,
	}
	cp, err := checkpoint.New(params)
	if err != nil {
		return checkpoint.Checkpoint{}, err
	}
	if err := repo.CreateCheckpoint(ctx, cp); err != nil {
		return checkpoint.Checkpoint{}, fmt.Errorf("create checkpoint: %w", err)
	}
	auditEventID, _ := idgen.NewID("audit")
	auditEvent, _ := audit.NewEvent(auditEventID, req.ActorID, "checkpoint.restored", "checkpoint", cp.ID, time.Now().UTC())
	auditEvent.Metadata = map[string]string{
		"sourceCheckpointId": sourceCP.ID,
		"targetSessionId":    req.TargetSessionID,
		"worldDirRel":        worldDirRel,
		"restoredRef":        agentResult.RestoredRef,
		"entryCount":         fmt.Sprintf("%d", agentResult.EntryCount),
		"sizeBytes":          fmt.Sprintf("%d", agentResult.SizeBytes),
		"checkpointId":       cp.ID,
		"projectId":          cp.ProjectID,
	}
	if err := repo.AppendAuditEvent(ctx, auditEvent); err != nil {
		return cp, fmt.Errorf("checkpoint restored but audit append failed: %w", err)
	}
	return cp, nil
}
