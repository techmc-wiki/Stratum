package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
)

func DryRunArtifactApply(ctx context.Context, runtimeRoot string, req agent.ArtifactApplyDryRunRequest, at time.Time) (agent.ArtifactApplyDryRunResult, error) {
	result := agent.ArtifactApplyDryRunResult{ApplyPlanID: req.ApplyPlanID, SessionID: req.SessionID, ArtifactID: req.ArtifactID, StagingPlanID: req.StagingPlanID, TargetRoot: req.TargetRoot, TargetRelativePath: req.TargetRelativePath, Action: "would_copy", Status: "not_ready", Issues: []string{}, CheckedAt: at}
	layout, manifest, err := readMaterializedArtifactManifest(ctx, runtimeRoot, req.SessionID)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("cannot read materialization manifest: %v", err))
		result.Status = "error"
		return result, nil
	}
	var entry *StagedRuntimeItem
	for _, item := range manifest.Items {
		if item.StagingPlanID == req.StagingPlanID {
			entry = &item
			break
		}
	}
	if entry == nil {
		result.Issues = append(result.Issues, "staging plan not in materialization manifest")
		result.Status = "not_ready"
		return result, nil
	}
	sourcePath := filepath.Join(layout.ArtifactsDir, entry.Path)
	if _, err := os.Stat(sourcePath); err != nil {
		result.Issues = append(result.Issues, "materialized artifact file missing")
		result.Status = "not_ready"
		return result, nil
	}
	hash, size, err := hashFile(sourcePath)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("cannot verify materialized artifact: %v", err))
		result.Status = "error"
		return result, nil
	}
	if hash != entry.PayloadHash {
		result.Issues = append(result.Issues, "materialized artifact hash mismatch")
		result.Status = "not_ready"
		return result, nil
	}
	if size != entry.PayloadSize {
		result.Issues = append(result.Issues, "materialized artifact size mismatch")
		result.Status = "not_ready"
		return result, nil
	}
	targetPath := filepath.Clean(req.TargetRelativePath)
	if filepath.IsAbs(targetPath) || strings.Contains(targetPath, "..") || targetPath == "." {
		result.Issues = append(result.Issues, "unsafe target path")
		result.Status = "error"
		return result, nil
	}
	targetRoot := mapTargetRoot(req.TargetRoot)
	if targetRoot == "" {
		result.Issues = append(result.Issues, "unsupported target root")
		result.Status = "error"
		return result, nil
	}
	plannedTarget := filepath.Join(targetRoot, targetPath)
	result.SourceRuntimeRelativePath = filepath.Join("artifacts", entry.Path)
	result.PlannedTargetRuntimeRelativePath = plannedTarget
	result.Status = "ready"
	return result, nil
}

func ExecuteArtifactApply(ctx context.Context, runtimeRoot string, req agent.ArtifactApplyExecuteRequest, at time.Time) (agent.ArtifactApplyExecuteResult, error) {
	result := agent.ArtifactApplyExecuteResult{ApplyPlanID: req.ApplyPlanID, SessionID: req.SessionID, ArtifactID: req.ArtifactID, StagingPlanID: req.StagingPlanID, TargetRoot: req.TargetRoot, TargetRelativePath: req.TargetRelativePath, Action: "copy", Status: "failed", Issues: []string{}, ExecutedAt: at}
	dryRunReq := agent.ArtifactApplyDryRunRequest{ApplyPlanID: req.ApplyPlanID, SessionID: req.SessionID, StagingPlanID: req.StagingPlanID, ArtifactID: req.ArtifactID, TargetRoot: req.TargetRoot, TargetRelativePath: req.TargetRelativePath, ExpectedHash: req.ExpectedHash, ExpectedSize: req.ExpectedSize}
	dryRun, err := DryRunArtifactApply(ctx, runtimeRoot, dryRunReq, at)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("dry-run failed: %v", err))
		return result, nil
	}
	if dryRun.Status != "ready" {
		result.Issues = append(result.Issues, "dry-run not ready")
		result.Issues = append(result.Issues, dryRun.Issues...)
		return result, nil
	}
	layout, _ := NewSessionRuntimeLayout(runtimeRoot, req.SessionID)
	sourcePath := filepath.Join(layout.SessionRoot, dryRun.SourceRuntimeRelativePath)
	targetRoot := mapTargetRootForExecution(req.TargetRoot, layout.WorkDir)
	if targetRoot == "" {
		result.Issues = append(result.Issues, "unsupported target root")
		return result, nil
	}
	targetPath := filepath.Join(targetRoot, filepath.Clean(req.TargetRelativePath))
	if !strings.HasPrefix(targetPath, targetRoot) {
		result.Issues = append(result.Issues, "computed target escapes target root")
		return result, nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("cannot create target directory: %v", err))
		return result, nil
	}
	src, err := os.Open(sourcePath)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("cannot open source: %v", err))
		return result, nil
	}
	defer src.Close()
	dst, err := os.Create(targetPath)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("cannot create target: %v", err))
		return result, nil
	}
	defer dst.Close()
	copied, err := io.Copy(dst, src)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("copy failed: %v", err))
		return result, nil
	}
	dst.Close()
	verifiedHash, _, err := hashFile(targetPath)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("cannot verify target: %v", err))
		return result, nil
	}
	if verifiedHash != req.ExpectedHash {
		result.Issues = append(result.Issues, "target hash mismatch after copy")
		return result, nil
	}
	result.SourcePath = sourcePath
	result.TargetPath = targetPath
	result.CopiedBytes = copied
	result.VerifiedTargetHash = verifiedHash
	result.Status = "applied"
	return result, nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func mapTargetRoot(root string) string {
	switch root {
	case "mods":
		return "work/mods"
	case "config":
		return "config"
	case "datapacks":
		return "work/datapacks"
	case "plugins":
		return "work/plugins"
	case "schematics":
		return "work/schematics"
	case "worlds":
		return "work/worlds"
	case "custom":
		return "work/custom"
	default:
		return ""
	}
}

func mapTargetRootForExecution(root, workDir string) string {
	switch root {
	case "mods":
		return filepath.Join(workDir, "mods")
	case "config":
		return filepath.Join(filepath.Dir(workDir), "config")
	case "datapacks":
		return filepath.Join(workDir, "datapacks")
	case "plugins":
		return filepath.Join(workDir, "plugins")
	case "schematics":
		return filepath.Join(workDir, "schematics")
	case "worlds":
		return filepath.Join(workDir, "worlds")
	case "custom":
		return filepath.Join(workDir, "custom")
	default:
		return ""
	}
}
