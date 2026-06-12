package operationsvc

import (
	"context"
	"testing"

	"github.com/stratummc/stratum/internal/domain/operation"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/repository/memory"
)

func TestBeginIdempotencyAndSessionConflict(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	service := New(store)
	params := BeginParams{IdempotencyKey: "key-1", ActorID: "actor-1", Action: "start", TargetType: "session", TargetID: "session-1", SessionID: "session-1"}
	first, replay, err := service.Begin(ctx, params)
	if err != nil || replay {
		t.Fatalf("begin: replay=%t err=%v", replay, err)
	}
	second, replay, err := service.Begin(ctx, params)
	if err != nil || !replay || second.ID != first.ID {
		t.Fatalf("replay: value=%+v replay=%t err=%v", second, replay, err)
	}
	_, _, err = service.Begin(ctx, BeginParams{ActorID: "actor-2", Action: "stop", TargetType: "session", TargetID: "session-1", SessionID: "session-1"})
	if !stratumerrors.IsKind(err, stratumerrors.KindConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	completed, err := service.Complete(ctx, first, operation.StatusSucceeded, "running", "success", "", "", nil)
	if err != nil || completed.Status != operation.StatusSucceeded {
		t.Fatalf("complete: %+v err=%v", completed, err)
	}
	values, err := store.ListOperations(ctx)
	if err != nil || len(values) != 1 || values[0].CompletedAt == nil {
		t.Fatalf("operations = %+v err=%v", values, err)
	}
}
