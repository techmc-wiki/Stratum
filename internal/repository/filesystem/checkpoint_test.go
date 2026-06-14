package filesystem

import (
	"context"
	"testing"

	"github.com/stratummc/stratum/internal/domain/checkpoint"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
)

func TestCheckpointMetadataOnlyCreate(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	cp := checkpoint.Checkpoint{
		ID: "cp-test", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", ProjectID: "p-1", RoomID: "r-1",
		CreatedAt: testTime,
	}
	if err := store.CreateCheckpoint(ctx, cp); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetCheckpoint(ctx, cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != cp.ID || got.Status != checkpoint.StatusMetadataOnly {
		t.Fatalf("got %+v", got)
	}
}

func TestCheckpointDuplicateIDFails(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	cp := checkpoint.Checkpoint{
		ID: "cp-dup", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	_ = store.CreateCheckpoint(ctx, cp)
	if err := store.CreateCheckpoint(ctx, cp); err == nil {
		t.Fatal("duplicate checkpoint id should fail")
	}
}

func TestCheckpointPersistsAfterReload(t *testing.T) {
	root := t.TempDir()
	store1, _ := New(root)
	ctx := context.Background()
	cp := checkpoint.Checkpoint{
		ID: "cp-persist", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	if err := store1.CreateCheckpoint(ctx, cp); err != nil {
		t.Fatal(err)
	}
	store2, _ := New(root)
	got, err := store2.GetCheckpoint(ctx, cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != cp.ID {
		t.Fatalf("got %+v", got)
	}
}

func TestCheckpointListFiltersEmpty(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	list, err := store.ListCheckpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
}

func TestCheckpointMissingSessionFails(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	cp := checkpoint.Checkpoint{
		ID: "cp-no-session", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	if err := store.CreateCheckpoint(ctx, cp); err == nil {
		t.Fatal("missing session should fail")
	}
}

func TestCheckpointMissingCreatorFails(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	cp := checkpoint.Checkpoint{
		ID: "cp-no-creator", SourceSessionID: "s-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	if err := store.CreateCheckpoint(ctx, cp); err == nil {
		t.Fatal("missing creator should fail")
	}
}

func TestCheckpointMissingEnvironmentIDFails(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	cp := checkpoint.Checkpoint{
		ID: "cp-no-env", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		CreatedAt: testTime,
	}
	if err := store.CreateCheckpoint(ctx, cp); err == nil {
		t.Fatal("metadata-only checkpoint without environment id should fail")
	}
}

func TestCheckpointUnsafeIDFails(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	cp := checkpoint.Checkpoint{
		ID: "../unsafe", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	if err := store.CreateCheckpoint(ctx, cp); err == nil {
		t.Fatal("unsafe checkpoint id should fail")
	}
}

func TestCheckpointNotFound(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	_, err := store.GetCheckpoint(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		t.Errorf("error kind = %v, want not found", err)
	}
}

func TestListCheckpointsBySessionReturnsOnlyMatching(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	cp1 := checkpoint.Checkpoint{
		ID: "cp-1", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	cp2 := checkpoint.Checkpoint{
		ID: "cp-2", SourceSessionID: "s-2", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	cp3 := checkpoint.Checkpoint{
		ID: "cp-3", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	_ = store.CreateCheckpoint(ctx, cp1)
	_ = store.CreateCheckpoint(ctx, cp2)
	_ = store.CreateCheckpoint(ctx, cp3)
	values, err := store.ListCheckpointsBySession(ctx, "s-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(values))
	}
	for _, cp := range values {
		if cp.SourceSessionID != "s-1" {
			t.Fatalf("expected session s-1, got %s", cp.SourceSessionID)
		}
	}
}

func TestListCheckpointsBySessionEmptyResult(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	cp := checkpoint.Checkpoint{
		ID: "cp-1", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	_ = store.CreateCheckpoint(ctx, cp)
	values, err := store.ListCheckpointsBySession(ctx, "s-nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("expected empty result, got %d", len(values))
	}
}

func TestListCheckpointsBySessionAfterReload(t *testing.T) {
	root := t.TempDir()
	store1, _ := New(root)
	ctx := context.Background()
	cp1 := checkpoint.Checkpoint{
		ID: "cp-1", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	cp2 := checkpoint.Checkpoint{
		ID: "cp-2", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	_ = store1.CreateCheckpoint(ctx, cp1)
	_ = store1.CreateCheckpoint(ctx, cp2)
	store2, _ := New(root)
	values, err := store2.ListCheckpointsBySession(ctx, "s-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(values))
	}
}

func TestListCheckpointsBySessionUnsafeIDFails(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	_, err := store.ListCheckpointsBySession(ctx, "../unsafe")
	if err == nil {
		t.Fatal("unsafe session id should fail")
	}
}
