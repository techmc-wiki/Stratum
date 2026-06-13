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
		return "mods"
	case "config":
		return "config"
	case "datapacks":
		return "world/datapacks"
	case "plugins":
		return "plugins"
	case "schematics":
		return "schematics"
	case "worlds":
		return "worlds"
	case "custom":
		return "custom"
	default:
		return ""
	}
}
