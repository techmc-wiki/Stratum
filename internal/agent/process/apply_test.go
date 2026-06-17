package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestResolvePathUnderRootNormalizesManifestSeparators(t *testing.T) {
	root := t.TempDir()
	got, err := resolvePathUnderRoot(root, `mods\nested/test.jar`)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "mods", "nested", "test.jar")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestResolvePathUnderRootRejectsAbsoluteAndTraversal(t *testing.T) {
	root := t.TempDir()
	for _, source := range []string{
		filepath.Join(root, "test.jar"),
		"C:/runtime/session/artifacts/test.jar",
		`C:\runtime\session\artifacts\test.jar`,
		"/runtime/session/artifacts/test.jar",
		"../test.jar",
		`..\test.jar`,
		"nested/../../test.jar",
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := resolvePathUnderRoot(root, source); err == nil {
				t.Fatalf("source path %q was accepted", source)
			}
		})
	}
}

func TestDryRunArtifactApplyUsesMaterializedSourceNotApplyTarget(t *testing.T) {
	root := t.TempDir()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	payload := []byte("materialized source")
	hash := "5c4da2ddc912f5df005c826d86957e3a0f4fd0d583cadd9bbfc600b10c804576"
	sourcePath := filepath.Join(layout.ArtifactsDir, "source", "artifact.jar")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, payload, 0o640); err != nil {
		t.Fatal(err)
	}
	entry := StagedRuntimeItem{ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", Name: "source/artifact.jar", Path: sourcePath, Kind: "artifact", PayloadHash: hash, PayloadSize: int64(len(payload)), CreatedAt: time.Now()}
	if err := layout.Staging().WriteArtifactManifest([]StagedRuntimeItem{entry}, time.Now()); err != nil {
		t.Fatal(err)
	}
	request := agent.ArtifactApplyDryRunRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", TargetRoot: "mods", TargetRelativePath: "different-target.jar"}
	result, err := DryRunArtifactApply(context.Background(), root, request, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ready" {
		t.Fatalf("status = %q, issues = %v", result.Status, result.Issues)
	}
	if result.SourceRuntimeRelativePath != "artifacts/source/artifact.jar" {
		t.Fatalf("source = %q", result.SourceRuntimeRelativePath)
	}
}

func TestExecuteArtifactApplyWithMaterializationManifestAbsolutePath(t *testing.T) {
	root := t.TempDir()
	materialization := materializationRequest([]byte("artifact"))
	if _, err := MaterializeArtifact(context.Background(), root, materialization, time.Now()); err != nil {
		t.Fatal(err)
	}
	request := agent.ArtifactApplyExecuteRequest{
		ApplyPlanID:        "apply-1",
		SessionID:          materialization.SessionID,
		StagingPlanID:      materialization.StagingPlanID,
		ArtifactID:         materialization.ArtifactID,
		TargetRoot:         "mods",
		TargetRelativePath: "applied.jar",
		ExpectedHash:       materialization.PayloadHash,
		ExpectedSize:       materialization.PayloadSize,
	}
	result, err := ExecuteArtifactApply(context.Background(), root, request, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" {
		t.Fatalf("status = %q, issues = %v", result.Status, result.Issues)
	}
	if _, err := os.Stat(result.TargetPath); err != nil {
		t.Fatalf("target missing: %v", err)
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

func TestExecuteArtifactApplyWritesManifest(t *testing.T) {
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
	if result.Status != "applied" {
		t.Fatalf("status = %q, want applied", result.Status)
	}
	records, err := ReadAppliedArtifacts(ctx, root, "session-1")
	if err != nil {
		t.Fatalf("cannot read applied artifacts: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	record := records[0]
	if record.ApplyPlanID != "apply-1" {
		t.Errorf("record.ApplyPlanID = %q, want apply-1", record.ApplyPlanID)
	}
	if record.ArtifactID != "artifact-1" {
		t.Errorf("record.ArtifactID = %q, want artifact-1", record.ArtifactID)
	}
	if record.Action != "copied" {
		t.Errorf("record.Action = %q, want copied", record.Action)
	}
}

func TestReadAppliedArtifactsEmptyList(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	records, err := ReadAppliedArtifacts(ctx, root, "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0", len(records))
	}
}

func TestReadAppliedArtifactNotFound(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	_, err := ReadAppliedArtifact(ctx, root, "session-1", "missing-plan")
	if !errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
		t.Errorf("error = %v, want ErrMaterializedArtifactNotFound", err)
	}
}

func TestVerifyAppliedArtifactValid(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	_ = os.MkdirAll(layout.ArtifactsDir, 0o755)
	_ = os.MkdirAll(filepath.Join(layout.SessionRoot, "work", "mods"), 0o755)
	staging := layout.Staging()
	payload := []byte("test artifact payload")
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", Name: "test.jar", Path: "test.jar", Kind: "artifact", PayloadHash: "81a05d74e71571648e1b5cd58e96cd346c87d0b94b8333099ee70b592fe87031", PayloadSize: int64(len(payload)), CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "test.jar"), payload, 0o640)
	req := agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", TargetRoot: "mods", TargetRelativePath: "test.jar", ExpectedHash: "81a05d74e71571648e1b5cd58e96cd346c87d0b94b8333099ee70b592fe87031"}
	_, err := ExecuteArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, err := VerifyAppliedArtifact(ctx, root, "session-1", "apply-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "valid" {
		t.Errorf("status = %q, want valid", result.Status)
	}
	if result.ExpectedHash != result.ActualHash {
		t.Errorf("hash mismatch: expected=%s, actual=%s", result.ExpectedHash, result.ActualHash)
	}
}

func TestVerifyAppliedArtifactMissing(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	_ = os.MkdirAll(layout.ArtifactsDir, 0o755)
	_ = os.MkdirAll(filepath.Join(layout.SessionRoot, "work", "mods"), 0o755)
	staging := layout.Staging()
	payload := []byte("test artifact payload")
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", Name: "test.jar", Path: "test.jar", Kind: "artifact", PayloadHash: "81a05d74e71571648e1b5cd58e96cd346c87d0b94b8333099ee70b592fe87031", PayloadSize: int64(len(payload)), CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "test.jar"), payload, 0o640)
	req := agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", TargetRoot: "mods", TargetRelativePath: "test.jar", ExpectedHash: "81a05d74e71571648e1b5cd58e96cd346c87d0b94b8333099ee70b592fe87031"}
	_, err := ExecuteArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = os.Remove(filepath.Join(layout.SessionRoot, "work", "mods", "test.jar"))
	result, err := VerifyAppliedArtifact(ctx, root, "session-1", "apply-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "missing" {
		t.Errorf("status = %q, want missing", result.Status)
	}
}

func TestVerifyAppliedArtifactCorrupted(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	_ = os.MkdirAll(layout.ArtifactsDir, 0o755)
	_ = os.MkdirAll(filepath.Join(layout.SessionRoot, "work", "mods"), 0o755)
	staging := layout.Staging()
	payload := []byte("test artifact payload")
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", Name: "test.jar", Path: "test.jar", Kind: "artifact", PayloadHash: "81a05d74e71571648e1b5cd58e96cd346c87d0b94b8333099ee70b592fe87031", PayloadSize: int64(len(payload)), CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "test.jar"), payload, 0o640)
	req := agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", TargetRoot: "mods", TargetRelativePath: "test.jar", ExpectedHash: "81a05d74e71571648e1b5cd58e96cd346c87d0b94b8333099ee70b592fe87031"}
	_, err := ExecuteArtifactApply(ctx, root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = os.WriteFile(filepath.Join(layout.SessionRoot, "work", "mods", "test.jar"), []byte("corrupted"), 0o640)
	result, err := VerifyAppliedArtifact(ctx, root, "session-1", "apply-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "corrupted" {
		t.Errorf("status = %q, want corrupted", result.Status)
	}
}

func TestVerifyAppliedArtifactNotFound(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	_, err := VerifyAppliedArtifact(ctx, root, "session-1", "missing-plan", time.Now())
	if !errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
		t.Errorf("error = %v, want ErrMaterializedArtifactNotFound", err)
	}
}

func TestVerifyAllAppliedArtifactsAllValid(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	_ = os.MkdirAll(layout.ArtifactsDir, 0o755)
	_ = os.MkdirAll(filepath.Join(layout.SessionRoot, "work", "mods"), 0o755)
	staging := layout.Staging()
	payload1 := []byte("artifact one")
	payload2 := []byte("artifact two")
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", Name: "one.jar", Path: "one.jar", Kind: "artifact", PayloadHash: "89e65e546593e5df88dee0a15c2db092d27c81a0a4ddad804680406cc82acc5b", PayloadSize: int64(len(payload1)), CreatedAt: time.Now()}, {ID: "item-2", StagingPlanID: "plan-2", ArtifactID: "artifact-2", Name: "two.jar", Path: "two.jar", Kind: "artifact", PayloadHash: "5d4b410d99c52f51d19c4c00b1bd8f34bff2c0c008d65fab3b168a8e159705bf", PayloadSize: int64(len(payload2)), CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "one.jar"), payload1, 0o640)
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "two.jar"), payload2, 0o640)
	res1, err1 := ExecuteArtifactApply(ctx, root, agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", TargetRoot: "mods", TargetRelativePath: "one.jar", ExpectedHash: "89e65e546593e5df88dee0a15c2db092d27c81a0a4ddad804680406cc82acc5b"}, time.Now())
	if err1 != nil || res1.Status != "applied" {
		t.Fatalf("apply1 failed: err=%v status=%s issues=%v", err1, res1.Status, res1.Issues)
	}
	res2, err2 := ExecuteArtifactApply(ctx, root, agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-2", SessionID: "session-1", StagingPlanID: "plan-2", ArtifactID: "artifact-2", TargetRoot: "mods", TargetRelativePath: "two.jar", ExpectedHash: "5d4b410d99c52f51d19c4c00b1bd8f34bff2c0c008d65fab3b168a8e159705bf"}, time.Now())
	if err2 != nil || res2.Status != "applied" {
		t.Fatalf("apply2 failed: err=%v status=%s issues=%v", err2, res2.Status, res2.Issues)
	}
	result, err := VerifyAllAppliedArtifacts(ctx, root, "session-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
	if result.ValidCount != 2 {
		t.Errorf("valid = %d, want 2", result.ValidCount)
	}
	if result.MissingCount != 0 || result.CorruptedCount != 0 || result.ErrorCount != 0 {
		t.Errorf("expected no missing/corrupted/error, got missing=%d corrupted=%d error=%d", result.MissingCount, result.CorruptedCount, result.ErrorCount)
	}
}

func TestVerifyAllAppliedArtifactsMixed(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	_ = os.MkdirAll(layout.ArtifactsDir, 0o755)
	_ = os.MkdirAll(filepath.Join(layout.SessionRoot, "work", "mods"), 0o755)
	staging := layout.Staging()
	payload1 := []byte("artifact one")
	payload2 := []byte("artifact two")
	payload3 := []byte("artifact three")
	_ = staging.WriteArtifactManifest([]StagedRuntimeItem{{ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", Name: "one.jar", Path: "one.jar", Kind: "artifact", PayloadHash: "89e65e546593e5df88dee0a15c2db092d27c81a0a4ddad804680406cc82acc5b", PayloadSize: int64(len(payload1)), CreatedAt: time.Now()}, {ID: "item-2", StagingPlanID: "plan-2", ArtifactID: "artifact-2", Name: "two.jar", Path: "two.jar", Kind: "artifact", PayloadHash: "5d4b410d99c52f51d19c4c00b1bd8f34bff2c0c008d65fab3b168a8e159705bf", PayloadSize: int64(len(payload2)), CreatedAt: time.Now()}, {ID: "item-3", StagingPlanID: "plan-3", ArtifactID: "artifact-3", Name: "three.jar", Path: "three.jar", Kind: "artifact", PayloadHash: "ca2e43652db828cd99ee7d93338fb698655a5cc780f0625eedbda0eef0791224", PayloadSize: int64(len(payload3)), CreatedAt: time.Now()}}, time.Now())
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "one.jar"), payload1, 0o640)
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "two.jar"), payload2, 0o640)
	_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "three.jar"), payload3, 0o640)
	_, _ = ExecuteArtifactApply(ctx, root, agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1", TargetRoot: "mods", TargetRelativePath: "one.jar", ExpectedHash: "89e65e546593e5df88dee0a15c2db092d27c81a0a4ddad804680406cc82acc5b"}, time.Now())
	_, _ = ExecuteArtifactApply(ctx, root, agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-2", SessionID: "session-1", StagingPlanID: "plan-2", ArtifactID: "artifact-2", TargetRoot: "mods", TargetRelativePath: "two.jar", ExpectedHash: "5d4b410d99c52f51d19c4c00b1bd8f34bff2c0c008d65fab3b168a8e159705bf"}, time.Now())
	_, _ = ExecuteArtifactApply(ctx, root, agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-3", SessionID: "session-1", StagingPlanID: "plan-3", ArtifactID: "artifact-3", TargetRoot: "mods", TargetRelativePath: "three.jar", ExpectedHash: "ca2e43652db828cd99ee7d93338fb698655a5cc780f0625eedbda0eef0791224"}, time.Now())
	_ = os.Remove(filepath.Join(layout.SessionRoot, "work", "mods", "two.jar"))
	_ = os.WriteFile(filepath.Join(layout.SessionRoot, "work", "mods", "three.jar"), []byte("corrupted"), 0o640)
	result, err := VerifyAllAppliedArtifacts(ctx, root, "session-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("total = %d, want 3", result.Total)
	}
	if result.ValidCount != 1 {
		t.Errorf("valid = %d, want 1", result.ValidCount)
	}
	if result.MissingCount != 1 {
		t.Errorf("missing = %d, want 1", result.MissingCount)
	}
	if result.CorruptedCount != 1 {
		t.Errorf("corrupted = %d, want 1", result.CorruptedCount)
	}
}

func TestVerifyAllAppliedArtifactsEmpty(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	result, err := VerifyAllAppliedArtifacts(ctx, root, "session-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0", result.Total)
	}
}

func TestExecuteArtifactApplyMCDRPluginToMCDRPluginsDir(t *testing.T) {
	root := t.TempDir()
	layout, _ := NewSessionRuntimeLayout(root, "session-mcdr-apply")
	_ = os.MkdirAll(layout.ArtifactsDir, 0o755)
	staging := layout.Staging()
	payload := []byte("mcdr plugin payload")
	hash := sha256hex(payload)
	if err := staging.WriteArtifactManifest([]StagedRuntimeItem{{
		ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1",
		Name: "test_plugin.py", Path: "test_plugin.py", Kind: "artifact",
		PayloadHash: hash, PayloadSize: int64(len(payload)), CreatedAt: time.Now(),
	}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.ArtifactsDir, "test_plugin.py"), payload, 0o640); err != nil {
		t.Fatal(err)
	}
	req := agent.ArtifactApplyExecuteRequest{
		ApplyPlanID: "apply-mcdr-1", SessionID: "session-mcdr-apply",
		StagingPlanID: "plan-1", ArtifactID: "artifact-1",
		TargetRoot: "mcdr_plugins", TargetRelativePath: "test_plugin.py",
		ExpectedHash: hash, ExpectedSize: int64(len(payload)),
	}
	result, err := ExecuteArtifactApply(context.Background(), root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "applied" {
		t.Fatalf("status = %q, want applied, issues = %v", result.Status, result.Issues)
	}
	expectedPath := filepath.Join(layout.WorkDir, "mcdr", "plugins", "test_plugin.py")
	if result.TargetPath != expectedPath {
		t.Fatalf("target = %q, want %q", result.TargetPath, expectedPath)
	}
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("mcdr plugin not at expected path: %v", err)
	}
	wrongPath := filepath.Join(layout.WorkDir, "plugins", "test_plugin.py")
	if _, err := os.Stat(wrongPath); err == nil {
		t.Error("mcdr_plugin target root should not write to work/plugins")
	}
	copied, _ := os.ReadFile(expectedPath)
	if string(copied) != string(payload) {
		t.Errorf("copied content mismatch")
	}
}

func TestDryRunArtifactApplyMCDRPluginResolvesMCDRPluginsDir(t *testing.T) {
	root := t.TempDir()
	layout, _ := NewSessionRuntimeLayout(root, "session-mcdr-dryrun")
	staging := layout.Staging()
	payload := []byte("mcdr plugin")
	hash := sha256hex(payload)
	if err := staging.WriteArtifactManifest([]StagedRuntimeItem{{
		ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1",
		Name: "plugin.py", Path: "plugin.py", Kind: "artifact",
		PayloadHash: hash, PayloadSize: int64(len(payload)), CreatedAt: time.Now(),
	}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.ArtifactsDir, "plugin.py"), payload, 0o640); err != nil {
		t.Fatal(err)
	}
	req := agent.ArtifactApplyDryRunRequest{
		ApplyPlanID: "apply-mcdr-dr-1", SessionID: "session-mcdr-dryrun",
		StagingPlanID: "plan-1", ArtifactID: "artifact-1",
		TargetRoot: "mcdr_plugins", TargetRelativePath: "plugin.py",
	}
	result, err := DryRunArtifactApply(context.Background(), root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "ready" {
		t.Fatalf("status = %q, want ready, issues = %v", result.Status, result.Issues)
	}
	if !strings.Contains(result.PlannedTargetRuntimeRelativePath, "mcdr") || !strings.Contains(result.PlannedTargetRuntimeRelativePath, "plugins") {
		t.Fatalf("planned target = %q, want path containing mcdr and plugins", result.PlannedTargetRuntimeRelativePath)
	}
}

func TestMapTargetRootMCDRPluginsDoesNotConflictWithOrdinaryPlugins(t *testing.T) {
	root := t.TempDir()
	layout, _ := NewSessionRuntimeLayout(root, "session-mixed")
	_ = os.MkdirAll(layout.ArtifactsDir, 0o755)
	staging := layout.Staging()
	payload := []byte("regular plugin")
	hash := sha256hex(payload)
	if err := staging.WriteArtifactManifest([]StagedRuntimeItem{{
		ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1",
		Name: "regular.jar", Path: "regular.jar", Kind: "artifact",
		PayloadHash: hash, PayloadSize: int64(len(payload)), CreatedAt: time.Now(),
	}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.ArtifactsDir, "regular.jar"), payload, 0o640); err != nil {
		t.Fatal(err)
	}
	mcdrPayload := []byte("mcdr plugin")
	mcdrHash := sha256hex(mcdrPayload)
	// apply regular plugin to "plugins" (work/plugins)
	regReq := agent.ArtifactApplyExecuteRequest{
		ApplyPlanID: "apply-regular", SessionID: "session-mixed",
		StagingPlanID: "plan-1", ArtifactID: "artifact-1",
		TargetRoot: "plugins", TargetRelativePath: "regular.jar",
		ExpectedHash: hash, ExpectedSize: int64(len(payload)),
	}
	regResult, err := ExecuteArtifactApply(context.Background(), root, regReq, time.Now())
	if err != nil || regResult.Status != "applied" {
		t.Fatalf("regular plugin apply failed: err=%v status=%s issues=%v", err, regResult.Status, regResult.Issues)
	}
	if _, err := os.Stat(filepath.Join(layout.WorkDir, "plugins", "regular.jar")); err != nil {
		t.Fatalf("regular plugin not at work/plugins: %v", err)
	}

	// apply mcdr plugin separately with staging plan for mcdr
	if err := staging.WriteArtifactManifest([]StagedRuntimeItem{{
		ID: "item-mcdr", StagingPlanID: "plan-mcdr", ArtifactID: "artifact-mcdr",
		Name: "mcdr_plugin.py", Path: "mcdr_plugin.py", Kind: "artifact",
		PayloadHash: mcdrHash, PayloadSize: int64(len(mcdrPayload)), CreatedAt: time.Now(),
	}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.ArtifactsDir, "mcdr_plugin.py"), mcdrPayload, 0o640); err != nil {
		t.Fatal(err)
	}
	mcdrReq := agent.ArtifactApplyExecuteRequest{
		ApplyPlanID: "apply-mcdr", SessionID: "session-mixed",
		StagingPlanID: "plan-mcdr", ArtifactID: "artifact-mcdr",
		TargetRoot: "mcdr_plugins", TargetRelativePath: "mcdr_plugin.py",
		ExpectedHash: mcdrHash, ExpectedSize: int64(len(mcdrPayload)),
	}
	mcdrResult, err := ExecuteArtifactApply(context.Background(), root, mcdrReq, time.Now())
	if err != nil || mcdrResult.Status != "applied" {
		t.Fatalf("mcdr plugin apply failed: err=%v status=%s issues=%v", err, mcdrResult.Status, mcdrResult.Issues)
	}
	mcdrExpected := filepath.Join(layout.WorkDir, "mcdr", "plugins", "mcdr_plugin.py")
	if _, err := os.Stat(mcdrExpected); err != nil {
		t.Fatalf("mcdr plugin not at expected path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.WorkDir, "plugins", "mcdr_plugin.py")); err == nil {
		t.Error("mcdr plugin leaked into work/plugins")
	}
}

func TestExecuteArtifactApplyMCDRPluginRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	layout, _ := NewSessionRuntimeLayout(root, "session-mcdr-traversal")
	_ = os.MkdirAll(layout.ArtifactsDir, 0o755)
	staging := layout.Staging()
	payload := []byte("bad plugin")
	hash := sha256hex(payload)
	if err := staging.WriteArtifactManifest([]StagedRuntimeItem{{
		ID: "item-1", StagingPlanID: "plan-1", ArtifactID: "artifact-1",
		Name: "plugin.py", Path: "plugin.py", Kind: "artifact",
		PayloadHash: hash, PayloadSize: int64(len(payload)), CreatedAt: time.Now(),
	}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.ArtifactsDir, "plugin.py"), payload, 0o640); err != nil {
		t.Fatal(err)
	}
	req := agent.ArtifactApplyExecuteRequest{
		ApplyPlanID: "apply-traversal", SessionID: "session-mcdr-traversal",
		StagingPlanID: "plan-1", ArtifactID: "artifact-1",
		TargetRoot: "mcdr_plugins", TargetRelativePath: "../escape.py",
		ExpectedHash: hash,
	}
	result, err := ExecuteArtifactApply(context.Background(), root, req, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status == "applied" {
		t.Error("path traversal should be rejected for mcdr_plugins target root")
	}
}

func sha256hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
