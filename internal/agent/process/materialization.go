package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
)

const materializedFilePermissions = 0o640

func MaterializeArtifact(ctx context.Context, runtimeRoot string, request agent.ArtifactMaterializationRequest, at time.Time) (agent.ArtifactMaterializationResult, error) {
	if request.SessionID == "" || request.ArtifactID == "" || request.StagingPlanID == "" || strings.TrimSpace(request.ActorID) == "" {
		return agent.ArtifactMaterializationResult{}, errors.New("session, artifact, staging plan, and actor are required")
	}
	if request.PayloadAlgorithm != "sha256" || !validMaterializationHash(request.PayloadHash) {
		return agent.ArtifactMaterializationResult{}, errors.New("materialization requires a valid SHA-256 payload")
	}
	if request.PayloadSize < 0 || request.PayloadSize != int64(len(request.Payload)) {
		return agent.ArtifactMaterializationResult{}, errors.New("materialization payload size does not match metadata")
	}
	if len(request.Payload) > agent.MaxArtifactPayloadBytes {
		return agent.ArtifactMaterializationResult{}, fmt.Errorf("materialization payload exceeds %d byte limit", agent.MaxArtifactPayloadBytes)
	}
	if err := ctx.Err(); err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	actualHash := sha256Hex(request.Payload)
	if actualHash != request.PayloadHash {
		return agent.ArtifactMaterializationResult{}, errors.New("materialization payload hash does not match metadata")
	}
	layout, err := NewSessionRuntimeLayout(runtimeRoot, request.SessionID)
	if err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	if err := rejectSymlinkPath(layout.RuntimeRoot, layout.ArtifactsDir); err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	if err := layout.Create(); err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	staging := layout.Staging()
	target, err := staging.ArtifactPath(request.TargetName)
	if err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	idempotent, err := writeMaterializedFile(ctx, staging.ArtifactsDir, target, request.Payload, request.PayloadHash)
	if err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	if err := updateArtifactManifest(staging, request, at); err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	relative, err := filepath.Rel(layout.SessionRoot, target)
	if err != nil {
		return agent.ArtifactMaterializationResult{}, fmt.Errorf("resolve materialized runtime path: %w", err)
	}
	return agent.ArtifactMaterializationResult{SessionID: request.SessionID, ArtifactID: request.ArtifactID, StagingPlanID: request.StagingPlanID, TargetName: request.TargetName, RuntimeRelativePath: filepath.ToSlash(relative), PayloadHash: request.PayloadHash, PayloadSize: request.PayloadSize, MaterializedAt: normalizeManifestTime(at), Idempotent: idempotent, Status: "materialized"}, nil
}

func writeMaterializedFile(ctx context.Context, artifactsRoot, target string, payload []byte, expectedHash string) (bool, error) {
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return false, errors.New("materialization target exists and is not a regular file")
		}
		hash, err := hashMaterializedFile(target)
		if err != nil {
			return false, err
		}
		if hash != expectedHash {
			return false, errors.New("materialization target already exists with different content")
		}
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect materialization target: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), runtimeDirectoryPermissions); err != nil {
		return false, fmt.Errorf("create materialization target directory: %w", err)
	}
	if err := rejectSymlinkPath(artifactsRoot, filepath.Dir(target)); err != nil {
		return false, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".stratum-artifact-*.tmp")
	if err != nil {
		return false, fmt.Errorf("create materialization temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(materializedFilePermissions); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("set materialized file permissions: %w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("write materialization payload: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = temporary.Close()
		return false, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("sync materialization payload: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close materialization payload: %w", err)
	}
	if err := os.Link(temporaryPath, target); err != nil {
		info, statErr := os.Lstat(target)
		if statErr == nil && info.Mode().IsRegular() {
			hash, hashErr := hashMaterializedFile(target)
			if hashErr != nil {
				return false, hashErr
			}
			if hash == expectedHash {
				return true, nil
			}
			return false, errors.New("materialization target already exists with different content")
		}
		return false, fmt.Errorf("commit materialization payload without overwrite: %w", err)
	}
	hash, err := hashMaterializedFile(target)
	if err != nil {
		return false, err
	}
	if hash != expectedHash {
		_ = os.Remove(target)
		return false, errors.New("materialized file failed SHA-256 verification")
	}
	return false, nil
}

func rejectSymlinkPath(root, candidate string) error {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	if !pathWithin(root, candidate) {
		return errors.New("materialization path escapes runtime root")
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return fmt.Errorf("resolve materialization path: %w", err)
	}
	current := root
	if info, err := os.Lstat(current); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("materialization path contains a symbolic link")
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect materialization path: %w", err)
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect materialization path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("materialization path contains a symbolic link")
		}
	}
	return nil
}

func updateArtifactManifest(staging SessionRuntimeStaging, request agent.ArtifactMaterializationRequest, at time.Time) error {
	manifest, err := staging.ReadArtifactManifest()
	if err != nil {
		return err
	}
	target, err := staging.ArtifactPath(request.TargetName)
	if err != nil {
		return err
	}
	entry := StagedRuntimeItem{ID: request.StagingPlanID, Name: request.TargetName, Path: target, Kind: "artifact", ArtifactID: request.ArtifactID, StagingPlanID: request.StagingPlanID, PayloadHash: request.PayloadHash, PayloadSize: request.PayloadSize, CreatedAt: normalizeManifestTime(at)}
	replaced := false
	for index := range manifest.Items {
		if manifest.Items[index].StagingPlanID == request.StagingPlanID || manifest.Items[index].ID == request.StagingPlanID {
			manifest.Items[index] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		manifest.Items = append(manifest.Items, entry)
	}
	sort.Slice(manifest.Items, func(i, j int) bool { return manifest.Items[i].ID < manifest.Items[j].ID })
	return staging.WriteArtifactManifest(manifest.Items, at)
}

func hashMaterializedFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open materialized file: %w", err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash materialized file: %w", err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func sha256Hex(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func validMaterializationHash(hash string) bool {
	if len(hash) != sha256.Size*2 {
		return false
	}
	for _, character := range hash {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
