package filesystem

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/operation"
)

func TestOperationPersistenceAndFilters(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "data")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	value := operation.Operation{ID: "operation-1", RequestID: "request-1", ActorID: "actor-1", Action: "start", TargetType: "session", TargetID: "session-1", SessionID: "session-1", Status: operation.StatusPending, CreatedAt: time.Now().UTC()}
	if err := store.CreateOperation(ctx, value); err != nil {
		t.Fatal(err)
	}
	value.Status = operation.StatusRunning
	if err := store.UpdateOperation(ctx, value); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.GetOperation(ctx, value.ID)
	if err != nil || got.Status != operation.StatusRunning {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	active, err := reloaded.ListActiveOperationsBySession(ctx, "session-1")
	if err != nil || len(active) != 1 {
		t.Fatalf("active=%+v err=%v", active, err)
	}
}
