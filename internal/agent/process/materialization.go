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
	"github.com/stratummc/stratum/internal/safepath"
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

func InspectMaterializedArtifacts(ctx context.Context, runtimeRoot, sessionID string) (agent.MaterializedArtifacts, error) {
	layout, manifest, err := readMaterializedArtifactManifest(ctx, runtimeRoot, sessionID)
	if err != nil {
		return agent.MaterializedArtifacts{}, err
	}
	staging := layout.Staging()
	items := make([]agent.MaterializedArtifact, 0, len(manifest.Items))
	for _, item := range manifest.Items {
		converted, err := inspectMaterializedArtifactItem(layout, staging, item)
		if err != nil {
			return agent.MaterializedArtifacts{}, err
		}
		items = append(items, converted)
	}
	status := "empty"
	if len(items) > 0 {
		status = "available"
	}
	return agent.MaterializedArtifacts{SessionID: sessionID, Status: status, Items: items}, nil
}

func readMaterializedArtifactManifest(ctx context.Context, runtimeRoot, sessionID string) (SessionRuntimeLayout, StagingManifest, error) {
	if err := ctx.Err(); err != nil {
		return SessionRuntimeLayout{}, StagingManifest{}, err
	}
	layout, err := NewSessionRuntimeLayout(runtimeRoot, sessionID)
	if err != nil {
		return SessionRuntimeLayout{}, StagingManifest{}, err
	}
	if err := rejectSymlinkPath(layout.RuntimeRoot, layout.ArtifactsDir); err != nil {
		return SessionRuntimeLayout{}, StagingManifest{}, err
	}
	staging := layout.Staging()
	if info, err := os.Lstat(staging.StagedArtifactsManifest); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return SessionRuntimeLayout{}, StagingManifest{}, errors.New("artifact staging manifest must not be a symbolic link")
	} else if err != nil && !os.IsNotExist(err) {
		return SessionRuntimeLayout{}, StagingManifest{}, fmt.Errorf("inspect artifact staging manifest: %w", err)
	}
	manifest, err := staging.ReadArtifactManifest()
	if err != nil {
		return SessionRuntimeLayout{}, StagingManifest{}, err
	}
	return layout, manifest, nil
}

func inspectMaterializedArtifactItem(layout SessionRuntimeLayout, staging SessionRuntimeStaging, item StagedRuntimeItem) (agent.MaterializedArtifact, error) {
	if item.Kind != "artifact" {
		return agent.MaterializedArtifact{}, fmt.Errorf("artifact staging manifest contains unsupported item kind %q", item.Kind)
	}
	if err := validateMaterializationIdentifier("staging plan", item.StagingPlanID); err != nil {
		return agent.MaterializedArtifact{}, err
	}
	target, err := staging.ArtifactPath(item.Name)
	if err != nil {
		return agent.MaterializedArtifact{}, fmt.Errorf("invalid materialized artifact target %q: %w", item.Name, err)
	}
	if item.Path != "" && filepath.Clean(item.Path) != filepath.Clean(target) {
		return agent.MaterializedArtifact{}, errors.New("materialized artifact manifest path does not match safe runtime path")
	}
	relative, err := filepath.Rel(layout.SessionRoot, target)
	if err != nil || !safepath.Within(layout.SessionRoot, target) {
		return agent.MaterializedArtifact{}, errors.New("materialized artifact path escapes session runtime")
	}
	return agent.MaterializedArtifact{
		ArtifactID: item.ArtifactID, StagingPlanID: item.StagingPlanID, ArtifactName: item.ArtifactName,
		ArtifactType: item.ArtifactType, TargetName: item.Name, PayloadAlgorithm: item.PayloadAlgorithm,
		PayloadHash: item.PayloadHash, PayloadSize: item.PayloadSize, RuntimeRelativePath: filepath.ToSlash(relative),
		MaterializedAt: item.CreatedAt, ActorID: item.ActorID, Status: item.Status, Metadata: cloneStringMap(item.Metadata),
	}, nil
}

func InspectMaterializedArtifact(ctx context.Context, runtimeRoot, sessionID, stagingPlanID string) (agent.MaterializedArtifact, error) {
	if err := validateMaterializationIdentifier("staging plan", stagingPlanID); err != nil {
		return agent.MaterializedArtifact{}, err
	}
	result, err := InspectMaterializedArtifacts(ctx, runtimeRoot, sessionID)
	if err != nil {
		return agent.MaterializedArtifact{}, err
	}
	for _, item := range result.Items {
		if item.StagingPlanID == stagingPlanID {
			item.SessionID = sessionID
			return item, nil
		}
	}
	return agent.MaterializedArtifact{}, fmt.Errorf("%w: staging plan %q in session %q", agent.ErrMaterializedArtifactNotFound, stagingPlanID, sessionID)
}

func VerifyMaterializedArtifact(ctx context.Context, runtimeRoot, sessionID, stagingPlanID string, at time.Time) (agent.MaterializedArtifactVerification, error) {
	item, err := InspectMaterializedArtifact(ctx, runtimeRoot, sessionID, stagingPlanID)
	if err != nil {
		return agent.MaterializedArtifactVerification{}, err
	}
	layout, err := NewSessionRuntimeLayout(runtimeRoot, sessionID)
	if err != nil {
		return agent.MaterializedArtifactVerification{}, err
	}
	return verifyMaterializedArtifactItem(ctx, layout, item, normalizeManifestTime(at))
}

func VerifyMaterializedArtifacts(ctx context.Context, runtimeRoot, sessionID string, at time.Time) (agent.MaterializedArtifactsVerification, error) {
	layout, manifest, err := readMaterializedArtifactManifest(ctx, runtimeRoot, sessionID)
	if err != nil {
		return agent.MaterializedArtifactsVerification{}, err
	}
	verifiedAt := normalizeManifestTime(at)
	result := agent.MaterializedArtifactsVerification{SessionID: sessionID, VerifiedAt: verifiedAt, Total: len(manifest.Items), Entries: make([]agent.MaterializedArtifactVerification, 0, len(manifest.Items))}
	staging := layout.Staging()
	for _, manifestItem := range manifest.Items {
		item, inspectErr := inspectMaterializedArtifactItem(layout, staging, manifestItem)
		if inspectErr != nil {
			entry := verificationErrorEntry(sessionID, manifestItem, verifiedAt, inspectErr)
			result.Entries = append(result.Entries, entry)
			result.ErrorCount++
			continue
		}
		entry, verifyErr := verifyMaterializedArtifactItem(ctx, layout, item, verifiedAt)
		if verifyErr != nil {
			entry = verificationErrorEntry(sessionID, manifestItem, verifiedAt, verifyErr)
			entry.RuntimeRelativePath = item.RuntimeRelativePath
			result.ErrorCount++
		} else {
			switch entry.Status {
			case "valid":
				result.ValidCount++
			case "missing":
				result.MissingCount++
			case "corrupted":
				result.CorruptedCount++
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func verifyMaterializedArtifactItem(ctx context.Context, layout SessionRuntimeLayout, item agent.MaterializedArtifact, verifiedAt time.Time) (agent.MaterializedArtifactVerification, error) {
	result := agent.MaterializedArtifactVerification{
		SessionID: layout.SessionID, StagingPlanID: item.StagingPlanID, ArtifactID: item.ArtifactID,
		TargetName: item.TargetName, RuntimeRelativePath: item.RuntimeRelativePath,
		PayloadAlgorithm: item.PayloadAlgorithm, ExpectedHash: item.PayloadHash,
		PayloadSize: item.PayloadSize, Status: "missing", VerifiedAt: verifiedAt,
	}
	if item.PayloadAlgorithm != "sha256" || !validMaterializationHash(item.PayloadHash) {
		return result, errors.New("materialized artifact manifest requires a valid SHA-256 payload hash")
	}
	staging := layout.Staging()
	target, err := staging.ArtifactPath(item.TargetName)
	if err != nil {
		return result, err
	}
	if err := rejectSymlinkPath(staging.ArtifactsDir, target); err != nil {
		return result, err
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return agent.MaterializedArtifactVerification{}, fmt.Errorf("inspect materialized artifact file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return agent.MaterializedArtifactVerification{}, errors.New("materialized artifact path is not a regular file")
	}
	actualHash, actualSize, err := hashMaterializedFileMetadata(ctx, target)
	if err != nil {
		return agent.MaterializedArtifactVerification{}, err
	}
	result.ActualHash = actualHash
	result.ActualSize = actualSize
	result.Status = "corrupted"
	if actualHash == item.PayloadHash && actualSize == item.PayloadSize {
		result.Status = "valid"
	}
	return result, nil
}

func verificationErrorEntry(sessionID string, item StagedRuntimeItem, verifiedAt time.Time, err error) agent.MaterializedArtifactVerification {
	return agent.MaterializedArtifactVerification{
		SessionID: sessionID, StagingPlanID: item.StagingPlanID, ArtifactID: item.ArtifactID,
		TargetName: item.Name, PayloadAlgorithm: item.PayloadAlgorithm, ExpectedHash: item.PayloadHash,
		PayloadSize: item.PayloadSize, Status: "error", VerifiedAt: verifiedAt, ErrorMessage: err.Error(),
	}
}

func validateMaterializationIdentifier(kind, value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("%s id is required", kind)
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return fmt.Errorf("%s id %q contains unsupported characters", kind, value)
	}
	return nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
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
	return safepath.RejectSymlinkPath(root, candidate, "materialization path")
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
	entry := StagedRuntimeItem{ID: request.StagingPlanID, Name: request.TargetName, Path: target, Kind: "artifact", ArtifactID: request.ArtifactID, StagingPlanID: request.StagingPlanID, ArtifactName: request.ArtifactName, ArtifactType: request.ArtifactType, PayloadAlgorithm: request.PayloadAlgorithm, PayloadHash: request.PayloadHash, PayloadSize: request.PayloadSize, ActorID: request.ActorID, Status: "materialized", CreatedAt: normalizeManifestTime(at)}
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
	hash, _, err := hashMaterializedFileMetadata(context.Background(), path)
	return hash, err
}

func hashMaterializedFileMetadata(ctx context.Context, path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open materialized file: %w", err)
	}
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, &contextReader{ctx: ctx, reader: file})
	if err != nil {
		return "", 0, fmt.Errorf("hash materialized file: %w", err)
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
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
