package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
)

func TestMaterializeArtifactWritesVerifiedFileAndManifest(t *testing.T) {
	root := t.TempDir()
	request := materializationRequest([]byte("artifact"))
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	result, err := MaterializeArtifact(context.Background(), root, request, now)
	if err != nil {
		t.Fatal(err)
	}
	layout, _ := NewSessionRuntimeLayout(root, request.SessionID)
	target := filepath.Join(layout.ArtifactsDir, "mods", "test.jar")
	payload, err := os.ReadFile(target)
	if err != nil || string(payload) != "artifact" || result.RuntimeRelativePath != filepath.ToSlash(filepath.Join("artifacts", "mods", "test.jar")) || result.PayloadHash != request.PayloadHash || result.MaterializedAt != now {
		t.Fatalf("result=%+v payload=%q err=%v", result, payload, err)
	}
	manifestFile, err := os.Open(layout.Staging().StagedArtifactsManifest)
	if err != nil {
		t.Fatal(err)
	}
	defer manifestFile.Close()
	var manifest StagingManifest
	if err := json.NewDecoder(manifestFile).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Items) != 1 || manifest.Items[0].ArtifactID != request.ArtifactID || manifest.Items[0].StagingPlanID != request.StagingPlanID || manifest.Items[0].PayloadHash != request.PayloadHash {
		t.Fatalf("manifest=%+v", manifest)
	}
}

func TestMaterializeArtifactIsIdempotentAndRejectsDifferentExistingContent(t *testing.T) {
	root := t.TempDir()
	request := materializationRequest([]byte("artifact"))
	if _, err := MaterializeArtifact(context.Background(), root, request, time.Now()); err != nil {
		t.Fatal(err)
	}
	result, err := MaterializeArtifact(context.Background(), root, request, time.Now())
	if err != nil || !result.Idempotent {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	layout, _ := NewSessionRuntimeLayout(root, request.SessionID)
	target, _ := layout.Staging().ArtifactPath(request.TargetName)
	if err := os.WriteFile(target, []byte("different"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializeArtifact(context.Background(), root, request, time.Now()); err == nil || !strings.Contains(err.Error(), "different content") {
		t.Fatalf("err=%v", err)
	}
}

func TestMaterializeArtifactRejectsUnsafeNameAndInvalidPayload(t *testing.T) {
	request := materializationRequest([]byte("artifact"))
	request.TargetName = "../escape.jar"
	if _, err := MaterializeArtifact(context.Background(), t.TempDir(), request, time.Now()); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("unsafe err=%v", err)
	}
	request = materializationRequest([]byte("artifact"))
	request.Payload[0] = 'X'
	if _, err := MaterializeArtifact(context.Background(), t.TempDir(), request, time.Now()); err == nil || !strings.Contains(err.Error(), "hash does not match") {
		t.Fatalf("hash err=%v", err)
	}
}

func TestMaterializeArtifactRejectsSymlinkedRuntimePath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sessions := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessions, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(sessions, "session-1")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if _, err := MaterializeArtifact(context.Background(), root, materializationRequest([]byte("artifact")), time.Now()); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "artifacts", "mods", "test.jar")); !os.IsNotExist(err) {
		t.Fatalf("materialization escaped runtime root: %v", err)
	}
}

func TestInspectMaterializedArtifacts(t *testing.T) {
	root := t.TempDir()
	request := materializationRequest([]byte("artifact"))
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	if _, err := MaterializeArtifact(context.Background(), root, request, now); err != nil {
		t.Fatal(err)
	}
	result, err := InspectMaterializedArtifacts(context.Background(), root, request.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "available" || len(result.Items) != 1 {
		t.Fatalf("result=%+v", result)
	}
	item := result.Items[0]
	if item.ArtifactID != request.ArtifactID || item.StagingPlanID != request.StagingPlanID || item.ArtifactName != request.ArtifactName || item.ArtifactType != request.ArtifactType || item.PayloadAlgorithm != request.PayloadAlgorithm || item.PayloadHash != request.PayloadHash || item.PayloadSize != request.PayloadSize || item.RuntimeRelativePath != "artifacts/mods/test.jar" || item.MaterializedAt != now || item.ActorID != request.ActorID || item.Status != "materialized" {
		t.Fatalf("item=%+v", item)
	}
}

func TestInspectMaterializedArtifactsMissingManifestIsEmpty(t *testing.T) {
	result, err := InspectMaterializedArtifacts(context.Background(), t.TempDir(), "session-1")
	if err != nil || result.Status != "empty" || len(result.Items) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestInspectMaterializedArtifactsRejectsUnsafeSessionAndMalformedManifest(t *testing.T) {
	if _, err := InspectMaterializedArtifacts(context.Background(), t.TempDir(), "../escape"); err == nil || !strings.Contains(err.Error(), "unsupported characters") {
		t.Fatalf("unsafe session err=%v", err)
	}
	root := t.TempDir()
	layout, err := NewSessionRuntimeLayout(root, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.Staging().StagedArtifactsManifest, []byte("not-json"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectMaterializedArtifacts(context.Background(), root, "session-1"); err == nil || !strings.Contains(err.Error(), "decode staging manifest") {
		t.Fatalf("malformed manifest err=%v", err)
	}
}

func TestInspectMaterializedArtifactByStagingPlan(t *testing.T) {
	root := t.TempDir()
	request := materializationRequest([]byte("artifact"))
	if _, err := MaterializeArtifact(context.Background(), root, request, time.Now()); err != nil {
		t.Fatal(err)
	}
	item, err := InspectMaterializedArtifact(context.Background(), root, request.SessionID, request.StagingPlanID)
	if err != nil || item.ArtifactID != request.ArtifactID || item.StagingPlanID != request.StagingPlanID || item.RuntimeRelativePath != "artifacts/mods/test.jar" {
		t.Fatalf("item=%+v err=%v", item, err)
	}
}

func TestInspectMaterializedArtifactNotFoundAndUnsafePlan(t *testing.T) {
	root := t.TempDir()
	if _, err := InspectMaterializedArtifact(context.Background(), root, "session-1", "plan-1"); !errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
		t.Fatalf("missing manifest err=%v", err)
	}
	request := materializationRequest([]byte("artifact"))
	if _, err := MaterializeArtifact(context.Background(), root, request, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectMaterializedArtifact(context.Background(), root, request.SessionID, "missing-plan"); !errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
		t.Fatalf("missing plan err=%v", err)
	}
	if _, err := InspectMaterializedArtifact(context.Background(), root, request.SessionID, "../escape"); err == nil || !strings.Contains(err.Error(), "unsupported characters") {
		t.Fatalf("unsafe plan err=%v", err)
	}
}

func TestVerifyMaterializedArtifactStatuses(t *testing.T) {
	root := t.TempDir()
	request := materializationRequest([]byte("artifact"))
	now := time.Date(2026, 6, 13, 14, 0, 0, 0, time.UTC)
	if _, err := MaterializeArtifact(context.Background(), root, request, now); err != nil {
		t.Fatal(err)
	}
	valid, err := VerifyMaterializedArtifact(context.Background(), root, request.SessionID, request.StagingPlanID, now)
	if err != nil || valid.Status != "valid" || valid.ExpectedHash != request.PayloadHash || valid.ActualHash != request.PayloadHash || valid.PayloadSize != request.PayloadSize || valid.ActualSize != request.PayloadSize || valid.VerifiedAt != now {
		t.Fatalf("valid=%+v err=%v", valid, err)
	}
	layout, _ := NewSessionRuntimeLayout(root, request.SessionID)
	target, _ := layout.Staging().ArtifactPath(request.TargetName)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	missing, err := VerifyMaterializedArtifact(context.Background(), root, request.SessionID, request.StagingPlanID, now)
	if err != nil || missing.Status != "missing" || missing.ActualHash != "" || missing.ActualSize != 0 {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
	if err := os.WriteFile(target, []byte("modified"), 0o640); err != nil {
		t.Fatal(err)
	}
	corrupted, err := VerifyMaterializedArtifact(context.Background(), root, request.SessionID, request.StagingPlanID, now)
	if err != nil || corrupted.Status != "corrupted" || corrupted.ActualHash == request.PayloadHash || corrupted.ActualSize != 8 {
		t.Fatalf("corrupted=%+v err=%v", corrupted, err)
	}
}

func TestVerifyMaterializedArtifactRejectsMissingUnsafeAndEscapingManifestPath(t *testing.T) {
	root := t.TempDir()
	if _, err := VerifyMaterializedArtifact(context.Background(), root, "session-1", "plan-1", time.Now()); !errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
		t.Fatalf("missing manifest err=%v", err)
	}
	request := materializationRequest([]byte("artifact"))
	if _, err := MaterializeArtifact(context.Background(), root, request, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyMaterializedArtifact(context.Background(), root, request.SessionID, "missing-plan", time.Now()); !errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
		t.Fatalf("missing entry err=%v", err)
	}
	if _, err := VerifyMaterializedArtifact(context.Background(), root, "../escape", request.StagingPlanID, time.Now()); err == nil || !strings.Contains(err.Error(), "unsupported characters") {
		t.Fatalf("unsafe session err=%v", err)
	}
	if _, err := VerifyMaterializedArtifact(context.Background(), root, request.SessionID, "../escape", time.Now()); err == nil || !strings.Contains(err.Error(), "unsupported characters") {
		t.Fatalf("unsafe plan err=%v", err)
	}
	layout, _ := NewSessionRuntimeLayout(root, request.SessionID)
	manifest := readManifest(t, layout.Staging().StagedArtifactsManifest)
	manifest.Items[0].Path = filepath.Join(t.TempDir(), "outside.jar")
	file, err := os.Create(layout.Staging().StagedArtifactsManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(file).Encode(manifest); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyMaterializedArtifact(context.Background(), root, request.SessionID, request.StagingPlanID, time.Now()); err == nil || !strings.Contains(err.Error(), "does not match safe runtime path") {
		t.Fatalf("escaping manifest path err=%v", err)
	}
}

func TestVerifyMaterializedArtifactsAllValidAndEmpty(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 13, 15, 0, 0, 0, time.UTC)
	for index, payload := range [][]byte{[]byte("artifact-one"), []byte("artifact-two")} {
		request := materializationRequest(payload)
		request.ArtifactID = fmt.Sprintf("artifact-%d", index+1)
		request.StagingPlanID = fmt.Sprintf("plan-%d", index+1)
		request.TargetName = fmt.Sprintf("mods/test-%d.jar", index+1)
		if _, err := MaterializeArtifact(context.Background(), root, request, now); err != nil {
			t.Fatal(err)
		}
	}
	result, err := VerifyMaterializedArtifacts(context.Background(), root, "session-1", now)
	if err != nil || result.Total != 2 || result.ValidCount != 2 || result.MissingCount != 0 || result.CorruptedCount != 0 || result.ErrorCount != 0 || len(result.Entries) != 2 || result.VerifiedAt != now {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	empty, err := VerifyMaterializedArtifacts(context.Background(), t.TempDir(), "session-1", now)
	if err != nil || empty.Total != 0 || len(empty.Entries) != 0 {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
}

func TestVerifyMaterializedArtifactsMixedStatusesAndEntryError(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 13, 16, 0, 0, 0, time.UTC)
	requests := make([]agent.ArtifactMaterializationRequest, 4)
	for index := range requests {
		payload := []byte(fmt.Sprintf("artifact-%d", index+1))
		request := materializationRequest(payload)
		request.ArtifactID = fmt.Sprintf("artifact-%d", index+1)
		request.StagingPlanID = fmt.Sprintf("plan-%d", index+1)
		request.TargetName = fmt.Sprintf("mods/test-%d.jar", index+1)
		requests[index] = request
		if _, err := MaterializeArtifact(context.Background(), root, request, now); err != nil {
			t.Fatal(err)
		}
	}
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	missingPath, _ := layout.Staging().ArtifactPath(requests[1].TargetName)
	if err := os.Remove(missingPath); err != nil {
		t.Fatal(err)
	}
	corruptedPath, _ := layout.Staging().ArtifactPath(requests[2].TargetName)
	if err := os.WriteFile(corruptedPath, []byte("corrupted"), 0o640); err != nil {
		t.Fatal(err)
	}
	manifest := readManifest(t, layout.Staging().StagedArtifactsManifest)
	manifest.Items[3].Path = filepath.Join(t.TempDir(), "outside.jar")
	writeManifest(t, layout.Staging().StagedArtifactsManifest, manifest)

	result, err := VerifyMaterializedArtifacts(context.Background(), root, "session-1", now)
	if err != nil || result.Total != 4 || result.ValidCount != 1 || result.MissingCount != 1 || result.CorruptedCount != 1 || result.ErrorCount != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	statuses := map[string]string{}
	for _, entry := range result.Entries {
		statuses[entry.StagingPlanID] = entry.Status
		if entry.Status == "error" && !strings.Contains(entry.ErrorMessage, "does not match safe runtime path") {
			t.Fatalf("error entry=%+v", entry)
		}
	}
	if statuses["plan-1"] != "valid" || statuses["plan-2"] != "missing" || statuses["plan-3"] != "corrupted" || statuses["plan-4"] != "error" {
		t.Fatalf("statuses=%v", statuses)
	}
}

func TestVerifyMaterializedArtifactsRejectsUnsafeSessionAndMalformedManifest(t *testing.T) {
	if _, err := VerifyMaterializedArtifacts(context.Background(), t.TempDir(), "../escape", time.Now()); err == nil || !strings.Contains(err.Error(), "unsupported characters") {
		t.Fatalf("unsafe session err=%v", err)
	}
	root := t.TempDir()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.Staging().StagedArtifactsManifest, []byte("not-json"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyMaterializedArtifacts(context.Background(), root, "session-1", time.Now()); err == nil || !strings.Contains(err.Error(), "decode staging manifest") {
		t.Fatalf("malformed manifest err=%v", err)
	}
}

func writeManifest(t *testing.T, path string, manifest StagingManifest) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(file).Encode(manifest); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func materializationRequest(payload []byte) agent.ArtifactMaterializationRequest {
	return agent.ArtifactMaterializationRequest{SessionID: "session-1", ArtifactID: "artifact-1", StagingPlanID: "plan-1", ArtifactName: "Test", ArtifactType: "jar", TargetName: "mods/test.jar", PayloadAlgorithm: "sha256", PayloadHash: sha256Hex(payload), PayloadSize: int64(len(payload)), ActorID: "actor-1", Payload: append([]byte(nil), payload...)}
}
