package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/checkpoint/consistency"
)

var testTime = time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

func TestMemoryListCheckpointsBySessionReturnsOnlyMatching(t *testing.T) {
	store := New()
	ctx := context.Background()
	cp1 := checkpoint.Checkpoint{
		ID: "cp-1", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly, ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	cp2 := checkpoint.Checkpoint{
		ID: "cp-2", SourceSessionID: "s-2", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly, ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	cp3 := checkpoint.Checkpoint{
		ID: "cp-3", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly, ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	store.Checkpoints[cp1.ID] = cp1
	store.Checkpoints[cp2.ID] = cp2
	store.Checkpoints[cp3.ID] = cp3
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

func TestMemoryListCheckpointsBySessionEmptyResult(t *testing.T) {
	store := New()
	ctx := context.Background()
	cp := checkpoint.Checkpoint{
		ID: "cp-1", SourceSessionID: "s-1", CreatorID: "u-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly, ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	store.Checkpoints[cp.ID] = cp
	values, err := store.ListCheckpointsBySession(ctx, "s-nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("expected empty result, got %d", len(values))
	}
}
