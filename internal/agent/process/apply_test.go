package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
)

func TestDryRunArtifactApplyReady(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	staging := layout.Staging()
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", Name: "test.jar", Path: "test.jar", Kind: "artifact", PayloadHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", PayloadSize: 0, CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "test.jar"), []byte{}, 0o640)
	req := agent.ArtifactApplyDryRunRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", TargetRoot: "mods", TargetRelativePath: "test.jar"}
	result, err := DryRunArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "ready" {
		t.Errorf("status = %q, want ready", result.Status)
	}
	if result.Action != "would_copy" {
		t.Errorf("action = %q, want would_copy", result.Action)
	}
	if len(result.Issues) > 0 {
		t.Errorf("expected no issues, got %v", result.Issues)
	}
}

func TestDryRunArtifactApplyMissingManifest(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	req := agent.ArtifactApplyDryRunRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", TargetRoot: "mods", TargetRelativePath: "test.jar"}
	result, err := DryRunArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status == "ready" {
		t.Errorf("status = %q, want not ready", result.Status)
	}
	if len(result.Issues) == 0 {
		t.Error("expected issues")
	}
}

func TestDryRunArtifactApplyMissingStagingPlan(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	staging := layout.Staging()
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-other", Name: "other.jar", Path: "other.jar", Kind: "artifact", CreatedAt: time.Now()}}, time.Now())
	req := agent.ArtifactApplyDryRunRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", TargetRoot: "mods", TargetRelativePath: "test.jar"}
	result, err := DryRunArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_ready" {
		t.Errorf("status = %q, want not_ready", result.Status)
	}
}

func TestDryRunArtifactApplyCorruptedFile(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	staging := layout.Staging()
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", Name: "test.jar", Path: "test.jar", Kind: "artifact", PayloadHash: "wronghash", PayloadSize: 10, CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "test.jar"), []byte{}, 0o640)
	req := agent.ArtifactApplyDryRunRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", TargetRoot: "mods", TargetRelativePath: "test.jar"}
	result, err := DryRunArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_ready" {
		t.Errorf("status = %q, want not_ready", result.Status)
	}
}

func TestDryRunArtifactApplyUnsafeTargetPath(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	staging := layout.Staging()
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", Name: "test.jar", Path: "test.jar", Kind: "artifact", PayloadHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", PayloadSize: 0, CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "test.jar"), []byte{}, 0o640)
	req := agent.ArtifactApplyDryRunRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", TargetRoot: "mods", TargetRelativePath: "../escape.jar"}
	result, err := DryRunArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "error" {
		t.Errorf("status = %q, want error", result.Status)
	}
}

func TestDryRunArtifactApplyDoesNotCreateFiles(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	staging := layout.Staging()
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", Name: "test.jar", Path: "test.jar", Kind: "artifact", PayloadHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", PayloadSize: 0, CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "test.jar"), []byte{}, 0o640)
	req := agent.ArtifactApplyDryRunRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", TargetRoot: "mods", TargetRelativePath: "test.jar"}
	_, _ = DryRunArtifactApply(ctx, root, req, time.Now())
	modsDir := filepath.Join(layout.RuntimeRoot, "mods")
	if _, err := os.Stat(modsDir); err == nil {
		t.Error("dry-run created mods directory")
	}
	targetFile := filepath.Join(modsDir, "test.jar")
	if _, err := os.Stat(targetFile); err == nil {
		t.Error("dry-run created target file")
	}
}
