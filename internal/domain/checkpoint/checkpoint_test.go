package checkpoint

import (
	"testing"
	"time"
)

func TestNewCheckpointMetadata(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	cp, err := New(CreateParams{ID: "cp-1", ProjectID: "p-1", RoomID: "r-1", SourceSessionID: "s-1", CreatorID: "u-1", Kind: KindManual, WorldStateRef: "worlds/immutable/snapshot-1", EnvironmentID: "env-117", LucyLockHash: "abc", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if cp.CreatedAt != now || cp.SourceSessionID != "s-1" || cp.WorldStateRef == "" {
		t.Fatalf("unexpected checkpoint: %+v", cp)
	}
}

func TestNewCheckpointRequiresWorldReference(t *testing.T) {
	_, err := New(CreateParams{ID: "cp-1", ProjectID: "p-1", SourceSessionID: "s-1", CreatorID: "u-1", EnvironmentID: "env-117"})
	if err == nil {
		t.Fatal("expected missing world state reference to fail")
	}
}
