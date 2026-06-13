package process

import (
	"context"
	"encoding/json"
	"errors"
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

func materializationRequest(payload []byte) agent.ArtifactMaterializationRequest {
	return agent.ArtifactMaterializationRequest{SessionID: "session-1", ArtifactID: "artifact-1", StagingPlanID: "plan-1", ArtifactName: "Test", ArtifactType: "jar", TargetName: "mods/test.jar", PayloadAlgorithm: "sha256", PayloadHash: sha256Hex(payload), PayloadSize: int64(len(payload)), ActorID: "actor-1", Payload: append([]byte(nil), payload...)}
}
