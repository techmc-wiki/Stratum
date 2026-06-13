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

func TestExecuteArtifactApplyCopiesFile(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	_ = os.MkdirAll(layout.ArtifactsDir, 0o755)
	staging := layout.Staging()
	payload := []byte("test artifact payload")
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", Name: "test.jar", Path: "test.jar", Kind: "artifact", PayloadHash: "81a05d74e71571648e1b5cd58e96cd346c87d0b94b8333099ee70b592fe87031", PayloadSize: int64(len(payload)), CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "test.jar"), payload, 0o640)
	req := agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", TargetRoot: "mods", TargetRelativePath: "test.jar", ExpectedHash: "81a05d74e71571648e1b5cd58e96cd346c87d0b94b8333099ee70b592fe87031"}
	result, err := ExecuteArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Issues) > 0 {
		t.Logf("Issues: %v", result.Issues)
	}
	if result.Status != "applied" {
		t.Errorf("status = %q, want applied", result.Status)
	}
	if result.Action != "copy" {
		t.Errorf("action = %q, want copy", result.Action)
	}
	if result.CopiedBytes != int64(len(payload)) {
		t.Errorf("copied %d bytes, want %d", result.CopiedBytes, len(payload))
	}
	if result.VerifiedTargetHash != "81a05d74e71571648e1b5cd58e96cd346c87d0b94b8333099ee70b592fe87031" {
		t.Errorf("hash = %q, want correct hash", result.VerifiedTargetHash)
	}
	if _, err := os.Stat(result.TargetPath); err != nil {
		t.Errorf("target file not created: %v", err)
	}
	copied, _ := os.ReadFile(result.TargetPath)
	if string(copied) != string(payload) {
		t.Errorf("target content = %q, want %q", copied, payload)
	}
}

func TestExecuteArtifactApplyFailsWhenDryRunNotReady(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	staging := layout.Staging()
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-other", Name: "other.jar", Path: "other.jar", Kind: "artifact", CreatedAt: time.Now()}}, time.Now())
	req := agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", TargetRoot: "mods", TargetRelativePath: "test.jar"}
	result, err := ExecuteArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "failed" {
		t.Errorf("status = %q, want failed", result.Status)
	}
	if len(result.Issues) == 0 {
		t.Error("expected issues")
	}
}

func TestExecuteArtifactApplyRejectsUnsafeTargetPath(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	staging := layout.Staging()
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", Name: "test.jar", Path: "test.jar", Kind: "artifact", PayloadHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", PayloadSize: 0, CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "test.jar"), []byte{}, 0o640)
	req := agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", TargetRoot: "mods", TargetRelativePath: "../escape.jar", ExpectedHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}
	result, err := ExecuteArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status == "applied" {
		t.Error("unsafe path should fail")
	}
}
