package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/checkpoint/consistency"
	"github.com/stratummc/stratum/internal/idgen"
	"github.com/stratummc/stratum/internal/resourcepolicy"
	sessionsvc "github.com/stratummc/stratum/internal/session/service"
	"github.com/stratummc/stratum/internal/storage/filesystem"
	stratumerrors "github.com/stratummc/stratum/internal/stratumerr"
)

func ensureResourcePolicy(ctx context.Context, store *filesystem.Store) (resourcepolicy.Policy, error) {
	value, err := store.GetResourcePolicy(ctx, "default")
	if err == nil {
		return value, nil
	}
	if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		return resourcepolicy.Policy{}, err
	}
	value = resourcepolicy.MVPDefault()
	if err := store.CreateResourcePolicy(ctx, value); err != nil {
		return resourcepolicy.Policy{}, err
	}
	return value, nil
}

func createPreOpCheckpoint(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, sessionID, actorID, notes string) error {
	sess, err := store.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session for pre-op checkpoint: %w", err)
	}
	snapResult, err := agentClient.CreateWorldSnapshot(ctx, agent.WorldCheckpointRequest{SessionID: sessionID})
	if err != nil {
		return fmt.Errorf("create world snapshot: %w", err)
	}
	cpID, idErr := idgen.NewID("cp")
	if idErr != nil {
		return fmt.Errorf("generate checkpoint id: %w", idErr)
	}
	if notes == "" {
		notes = "Pre-operation checkpoint"
	}
	cp := checkpoint.Checkpoint{
		ID:               cpID,
		ProjectID:        sess.ProjectID,
		RoomID:           sess.RoomID,
		SourceSessionID:  sessionID,
		CreatorID:        actorID,
		Kind:             checkpoint.KindPreOperation,
		Status:           checkpoint.StatusMetadataOnly,
		ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID:    sess.EnvironmentID,
		RuntimeProfileID: sess.RuntimeProfileID,
		WorldStateRef:    snapResult.SnapshotRef,
		Notes:            notes,
		CreatedAt:        time.Now().UTC(),
	}
	if err := store.CreateCheckpoint(ctx, cp); err != nil {
		return fmt.Errorf("save pre-op checkpoint: %w", err)
	}
	return nil
}

func makePreOpSessionCheckpointFunc(store *filesystem.Store, agentClient agent.AgentClient) sessionsvc.PreOpCheckpointFunc {
	return func(ctx context.Context, sessionID, actorID string) error {
		return createPreOpCheckpoint(ctx, store, agentClient, sessionID, actorID, "Pre-operation checkpoint before session restart")
	}
}
