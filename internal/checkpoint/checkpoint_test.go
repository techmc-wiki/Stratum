package checkpoint

import (
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/checkpoint/consistency"
)

func TestNewMetadataOnlyCheckpoint(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	cp, err := New(CreateParams{ID: "cp-1", ProjectID: "p-1", RoomID: "r-1", SourceSessionID: "s-1", CreatorID: "u-1", Kind: KindManual, Status: StatusMetadataOnly, EnvironmentID: "env-117", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if cp.CreatedAt != now || cp.SourceSessionID != "s-1" || cp.Status != StatusMetadataOnly || cp.ConsistencyLevel != consistency.LevelMetadataOnly {
		t.Fatalf("got %+v", cp)
	}
}

func TestNewCheckpointStoresConsistencyLevel(t *testing.T) {
	cp, err := New(CreateParams{
		ID: "cp-1", SourceSessionID: "s-1", CreatorID: "u-1", Kind: KindManual,
		Status: StatusMetadataOnly, ConsistencyLevel: consistency.LevelPluginBackup,
		ConsistencyMetadata: map[string]string{"backupId": "backup-1"}, EnvironmentID: "env-117",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.ConsistencyLevel != consistency.LevelPluginBackup || cp.ConsistencyMetadata["backupId"] != "backup-1" {
		t.Fatalf("consistency fields = %+v", cp)
	}
}

func TestNewCheckpointRejectsUnknownConsistencyLevel(t *testing.T) {
	_, err := New(CreateParams{ID: "cp-1", SourceSessionID: "s-1", CreatorID: "u-1", Status: StatusMetadataOnly, ConsistencyLevel: consistency.Level("unknown"), EnvironmentID: "env-117"})
	if err == nil {
		t.Fatal("expected unknown consistency level to fail")
	}
}

func TestNewMetadataOnlyCheckpointRequiresEnvironmentID(t *testing.T) {
	_, err := New(CreateParams{ID: "cp-1", SourceSessionID: "s-1", CreatorID: "u-1", Status: StatusMetadataOnly})
	if err == nil {
		t.Fatal("expected missing environment id to fail")
	}
}

func TestNewCheckpointRequiresSessionAndCreator(t *testing.T) {
	_, err := New(CreateParams{ID: "cp-1"})
	if err == nil {
		t.Fatal("expected missing session/creator to fail")
	}
}
