package checkpointsvc

import (
	"context"
	"fmt"
	"time"

	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/checkpoint"
	"github.com/stratummc/stratum/internal/domain/session"
	"github.com/stratummc/stratum/internal/util"
)

type CreateRequest struct {
	ID                    string
	SessionID             string
	ActorID               string
	Notes                 string
	RuntimeStatusSnapshot *checkpoint.RuntimeStatusSnapshot
}

type SessionReader interface {
	GetSession(ctx context.Context, id string) (session.Session, error)
}

type Repository interface {
	SessionReader
	CreateCheckpoint(ctx context.Context, cp checkpoint.Checkpoint) error
	GetCheckpoint(ctx context.Context, id string) (checkpoint.Checkpoint, error)
	ListCheckpoints(ctx context.Context) ([]checkpoint.Checkpoint, error)
	ListCheckpointsBySession(ctx context.Context, sessionID string) ([]checkpoint.Checkpoint, error)
	AppendAuditEvent(ctx context.Context, event audit.Event) error
}

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
	cp, err := checkpoint.New(checkpoint.CreateParams{
		ID:                    req.ID,
		ProjectID:             sess.ProjectID,
		RoomID:                sess.RoomID,
		SourceSessionID:       sess.ID,
		CreatorID:             req.ActorID,
		Kind:                  checkpoint.KindManual,
		Status:                checkpoint.StatusMetadataOnly,
		EnvironmentID:         sess.EnvironmentID,
		RuntimeProfileID:      sess.RuntimeProfileID,
		RuntimeStatusSnapshot: prepareRuntimeStatusSnapshot(req.RuntimeStatusSnapshot, sess),
		Notes:                 req.Notes,
	})
	if err != nil {
		return checkpoint.Checkpoint{}, err
	}
	if err := repo.CreateCheckpoint(ctx, cp); err != nil {
		return checkpoint.Checkpoint{}, err
	}
	eventID, _ := util.NewID("audit")
	event, _ := audit.NewEvent(eventID, req.ActorID, "checkpoint.created", "checkpoint", cp.ID, time.Now().UTC())
	event.Metadata = map[string]string{
		"checkpointId":                  cp.ID,
		"projectId":                     cp.ProjectID,
		"roomId":                        cp.RoomID,
		"sessionId":                     cp.SourceSessionID,
		"environmentId":                 cp.EnvironmentID,
		"status":                        string(cp.Status),
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
	_ = repo.AppendAuditEvent(ctx, event)
	return cp, nil
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
