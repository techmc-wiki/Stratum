package checkpointsvc

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/checkpoint"
	"github.com/stratummc/stratum/internal/util"
)

type CreateRequest struct {
	ID        string
	SessionID string
	ActorID   string
	Notes     string
}

type Repository interface {
	GetSession(ctx context.Context, id string) (interface{}, error)
	CreateCheckpoint(ctx context.Context, cp checkpoint.Checkpoint) error
	GetCheckpoint(ctx context.Context, id string) (checkpoint.Checkpoint, error)
	ListCheckpoints(ctx context.Context) ([]checkpoint.Checkpoint, error)
	ListCheckpointsBySession(ctx context.Context, sessionID string) ([]checkpoint.Checkpoint, error)
	AppendAuditEvent(ctx context.Context, event audit.Event) error
}

type sessionData struct {
	ID            string
	ProjectID     string
	RoomID        string
	EnvironmentID string
}

func Create(ctx context.Context, repo Repository, req CreateRequest) (checkpoint.Checkpoint, error) {
	if req.ActorID == "" {
		return checkpoint.Checkpoint{}, fmt.Errorf("actor required")
	}
	sessRaw, err := repo.GetSession(ctx, req.SessionID)
	if err != nil {
		return checkpoint.Checkpoint{}, err
	}
	sess := extractSessionData(sessRaw)
	cp, err := checkpoint.New(checkpoint.CreateParams{
		ID:              req.ID,
		ProjectID:       sess.ProjectID,
		RoomID:          sess.RoomID,
		SourceSessionID: sess.ID,
		CreatorID:       req.ActorID,
		Kind:            checkpoint.KindManual,
		Status:          checkpoint.StatusMetadataOnly,
		EnvironmentID:   sess.EnvironmentID,
		Notes:           req.Notes,
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
		"checkpointId":  cp.ID,
		"projectId":     cp.ProjectID,
		"roomId":        cp.RoomID,
		"sessionId":     cp.SourceSessionID,
		"environmentId": cp.EnvironmentID,
		"status":        string(cp.Status),
		"actor":         req.ActorID,
	}
	_ = repo.AppendAuditEvent(ctx, event)
	return cp, nil
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

func extractSessionData(sessRaw interface{}) sessionData {
	v := reflect.ValueOf(sessRaw)
	return sessionData{
		ID:            v.FieldByName("ID").String(),
		ProjectID:     v.FieldByName("ProjectID").String(),
		RoomID:        v.FieldByName("RoomID").String(),
		EnvironmentID: v.FieldByName("EnvironmentID").String(),
	}
}
