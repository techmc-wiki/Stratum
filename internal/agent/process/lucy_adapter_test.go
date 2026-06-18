package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/integration/lucy"
)

func TestDefaultLucyAdapterIsNoop(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-noop",
		EnvironmentID:    "env-117-fabric",
		EnvironmentName:  "1.17 Fabric Carpet",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		LoaderVersion:    "0.12.0",
		ServerCore:       "carpet",
		MCDRRequired:     true,
		CarpetRequired:   true,
		ActorID:          "alice",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Metadata["lucyAdapterMode"] != "noop" {
		t.Errorf("lucy adapter mode: got %q, want %q", result.Metadata["lucyAdapterMode"], "noop")
	}
	if result.Metadata["lucyResolutionStatus"] != "not_requested" {
		t.Errorf("lucy resolution status: got %q, want %q", result.Metadata["lucyResolutionStatus"], "not_requested")
	}
	if result.Metadata["lucyAdapterConfigured"] != "false" {
		t.Errorf("lucy adapter configured: got %q, want %q", result.Metadata["lucyAdapterConfigured"], "false")
	}
}

func TestDetectLucyAdapterNoop(t *testing.T) {
	t.Setenv("STRATUM_LUCY_WORKSPACE", "")
	adapter := detectLucyAdapter(filepath.Join(t.TempDir(), "missing"))
	if _, ok := adapter.(lucy.NoopAdapter); !ok {
		t.Fatalf("adapter = %T, want lucy.NoopAdapter", adapter)
	}
}

func TestDetectLucyAdapterEmbedded(t *testing.T) {
	t.Setenv("STRATUM_LUCY_WORKSPACE", "")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "lucy.yaml"), []byte("format_version: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := detectLucyAdapter(root)
	if _, ok := adapter.(*lucy.EmbeddedAdapter); !ok {
		t.Fatalf("adapter = %T, want *lucy.EmbeddedAdapter", adapter)
	}
}

func TestDetectLucyAdapterEnvNone(t *testing.T) {
	t.Setenv("STRATUM_LUCY_WORKSPACE", "none")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "lucy.yaml"), []byte("format_version: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := detectLucyAdapter(root)
	if _, ok := adapter.(lucy.NoopAdapter); !ok {
		t.Fatalf("adapter = %T, want lucy.NoopAdapter", adapter)
	}
}

func TestSetLucyAdapterNilDefaultsToNoop(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(nil)
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-nil",
		EnvironmentID:    "env-117-fabric",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		ServerCore:       "carpet",
		ActorID:          "alice",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Metadata["lucyAdapterMode"] != "noop" {
		t.Errorf("lucy adapter mode: got %q, want %q", result.Metadata["lucyAdapterMode"], "noop")
	}
}

func TestSetLucyAdapterEmbedded(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-embedded",
		EnvironmentID:    "env-117-fabric",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		ServerCore:       "carpet",
		ActorID:          "alice",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Metadata["lucyAdapterMode"] != "embedded" {
		t.Errorf("lucy adapter mode: got %q, want %q", result.Metadata["lucyAdapterMode"], "embedded")
	}
	if result.Metadata["lucyAdapterConfigured"] != "true" {
		t.Errorf("lucy adapter configured: got %q, want %q", result.Metadata["lucyAdapterConfigured"], "true")
	}
	if result.LucyResolutionStatus != "resolved" {
		t.Errorf("lucy resolution status: got %q, want resolved", result.LucyResolutionStatus)
	}
}

func TestMaterializationWritesManifestWithLucyMetadata(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-manifest",
		EnvironmentID:    "env-117-fabric",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		ServerCore:       "carpet",
		ActorID:          "alice",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	manifestPath := result.Metadata["manifestPath"]
	if manifestPath == "" {
		t.Fatal("manifest path not in metadata")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest["lucy_adapter_mode"] != "noop" {
		t.Errorf("manifest lucy_adapter_mode: got %v, want noop", manifest["lucy_adapter_mode"])
	}
	if manifest["lucy_resolution_status"] != "not_requested" {
		t.Errorf("manifest lucy_resolution_status: got %v, want not_requested", manifest["lucy_resolution_status"])
	}
}

func TestMaterializationDoesNotWriteLucyManifests(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-no-lucy",
		EnvironmentID:    "env-117-fabric",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		ServerCore:       "carpet",
		ActorID:          "alice",
	}
	_, err = supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-no-lucy")
	lucyYaml := filepath.Join(sessionRoot, "config", "lucy.yaml")
	if _, err := os.Stat(lucyYaml); err == nil {
		t.Error("lucy.yaml should not exist")
	}
	lucyLock := filepath.Join(sessionRoot, "config", "lucy-lock.yaml")
	if _, err := os.Stat(lucyLock); err == nil {
		t.Error("lucy-lock.yaml should not exist")
	}
}

func TestMaterializeEnvironmentWithEmbeddedAdapterResolvesLock(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{
		plan: lucy.EnvironmentPlan{
			Actions: []lucy.PlanAction{
				{ActionType: lucy.ActionDownload, PackageID: "fabric-api", Target: "mods/fabric-api.jar", Hash: "abc", Size: 1},
			},
			Metadata: map[string]string{},
		},
		lock: lucy.EnvironmentLock{
			LockID:           "lock-1",
			LockHash:         "sha256:lockhash",
			GeneratedAt:      time.Now().UTC(),
			ProviderMetadata: map[string]string{},
		},
	}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)

	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-resolved"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Status != "prepared" {
		t.Errorf("status: got %q, want prepared", result.Status)
	}
	if result.LucyResolutionStatus != "resolved" {
		t.Errorf("lucy status: got %q, want resolved", result.LucyResolutionStatus)
	}
	if result.LucyLockHash != "sha256:lockhash" {
		t.Errorf("lock hash: got %q, want sha256:lockhash", result.LucyLockHash)
	}
	lockPath := filepath.Join(root, "sessions", "session-resolved", "config", "lucy-lock.yaml")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lucy-lock.yaml not written: %v", err)
	}
	manifestPath := result.Metadata["manifestPath"]
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read materialization manifest: %v", err)
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal materialization manifest: %v", err)
	}
	if manifest["lucyResolutionStatus"] != "resolved" {
		t.Errorf("manifest lucyResolutionStatus: got %v, want resolved", manifest["lucyResolutionStatus"])
	}
}

func TestMaterializeEnvironmentWithEmbeddedAdapterPlanErrorDegradesGracefully(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := lucy.NewEmbeddedAdapter(&fakeBackend{err: errors.New("planner unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)

	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-plan-error"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Status != "prepared" {
		t.Errorf("status: got %q, want prepared", result.Status)
	}
	if result.LucyResolutionStatus != "failed" {
		t.Errorf("lucy status: got %q, want failed", result.LucyResolutionStatus)
	}
	if result.Metadata["lucyResolutionError"] == "" {
		t.Error("expected lucyResolutionError metadata")
	}
	lockPath := filepath.Join(root, "sessions", "session-plan-error", "config", "lucy-lock.yaml")
	if _, err := os.Stat(lockPath); err == nil {
		t.Fatal("lucy-lock.yaml should not be written after plan failure")
	}
}

func TestMaterializeEnvironmentWithNoopAdapterStillDoesNotResolve(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-noop-stable"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.LucyResolutionStatus != "not_requested" {
		t.Errorf("lucy status: got %q, want not_requested", result.LucyResolutionStatus)
	}
	if result.LucyLockHash != "" || result.LucyManifestPath != "" || result.LucyLockPath != "" {
		t.Errorf("noop result should not include lucy paths or lock hash: %#v", result)
	}
}

func TestMaterializeEnvironmentWritesLucyManifest(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := lucy.NewEmbeddedAdapter(&fakeBackend{
		plan: lucy.EnvironmentPlan{Metadata: map[string]string{}},
		lock: lucy.EnvironmentLock{LockID: "lock-1", LockHash: "hash", GeneratedAt: time.Now().UTC(), ProviderMetadata: map[string]string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)

	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-lucy-manifest"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	lucyManifestPath := filepath.Join(root, result.LucyManifestPath)
	if _, err := os.Stat(lucyManifestPath); err != nil {
		t.Fatalf("lucy.yaml not written: %v", err)
	}
	data, err := os.ReadFile(lucyManifestPath)
	if err != nil {
		t.Fatalf("read lucy.yaml: %v", err)
	}
	if !json.Valid(data) && len(data) == 0 {
		t.Fatal("lucy.yaml should not be empty")
	}
}

func TestMaterializeEnvironmentWithEmbeddedAdapterInstallsPackages(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("carpet jar")
	hashBytes := sha256.Sum256(content)
	hash := hex.EncodeToString(hashBytes[:])
	backend := &fakeBackend{
		plan: lucy.EnvironmentPlan{Metadata: map[string]string{}},
		lock: lucy.EnvironmentLock{
			LockID:      "lock-1",
			LockHash:    "lockhash",
			GeneratedAt: time.Now().UTC(),
			Packages: []lucy.LockedPackage{
				{ID: "fabric/carpet", Source: "modrinth", Name: "carpet", Version: "1.4.83", Hash: hash, Size: int64(len(content))},
			},
			ProviderMetadata: map[string]string{},
		},
		installContent: content,
	}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)

	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-install-ok"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Status != "prepared" {
		t.Errorf("status: got %q, want prepared", result.Status)
	}
	if result.LucyInstallStatus != "ok" {
		t.Errorf("install status: got %q, want ok", result.LucyInstallStatus)
	}
	if result.LucyInstalledCount != 1 || result.LucyFailedCount != 0 {
		t.Errorf("install counts: installed=%d failed=%d", result.LucyInstalledCount, result.LucyFailedCount)
	}
	modPath := filepath.Join(root, "sessions", "session-install-ok", "mods", "carpet-1.4.83.jar")
	if _, err := os.Stat(modPath); err != nil {
		t.Fatalf("mod file not written: %v", err)
	}
	manifestPath := result.Metadata["manifestPath"]
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest["lucyInstallStatus"] != "ok" {
		t.Errorf("manifest install status: got %v, want ok", manifest["lucyInstallStatus"])
	}
}

func TestMaterializeEnvironmentWithEmbeddedAdapterInstallFailsGracefully(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{
		plan: lucy.EnvironmentPlan{Metadata: map[string]string{}},
		lock: lucy.EnvironmentLock{
			LockID:      "lock-1",
			LockHash:    "lockhash",
			GeneratedAt: time.Now().UTC(),
			Packages: []lucy.LockedPackage{
				{ID: "fabric/carpet", Source: "modrinth", Name: "carpet", Version: "1.4.83", Hash: "abc", Size: 1},
			},
			ProviderMetadata: map[string]string{},
		},
		installErr: errors.New("download failed"),
	}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)

	_, err = supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-install-fail"))
	if err == nil {
		t.Fatal("expected integrity error after failed install")
	}
	var integrityErr *agent.EnvironmentIntegrityError
	if !errors.As(err, &integrityErr) {
		t.Fatalf("expected EnvironmentIntegrityError, got %T: %v", err, err)
	}
	if integrityErr.Status != "missing_files" {
		t.Fatalf("integrity status: got %q, want missing_files", integrityErr.Status)
	}
}

func TestMaterializeEnvironmentWithNoopAdapterSkipsInstall(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-install-noop"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.LucyInstallStatus != "" {
		t.Errorf("install status: got %q, want empty", result.LucyInstallStatus)
	}
	entries, err := os.ReadDir(filepath.Join(root, "sessions", "session-install-noop", "mods"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("mods dir should be empty, got %d entries", len(entries))
	}
}

func TestMaterializeEnvironmentWithEmptyLockSkipsInstall(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{
		plan: lucy.EnvironmentPlan{Metadata: map[string]string{}},
		lock: lucy.EnvironmentLock{LockID: "lock-1", LockHash: "lockhash", GeneratedAt: time.Now().UTC(), ProviderMetadata: map[string]string{}},
	}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)
	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-empty-lock"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.LucyInstallStatus != "" {
		t.Errorf("install status: got %q, want empty", result.LucyInstallStatus)
	}
	if backend.installCalled {
		t.Error("InstallPackages should not be called for empty lock")
	}
}

func TestMaterializeEnvironmentWithIntegrityPass(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("valid jar")
	hashBytes := sha256.Sum256(content)
	hash := hex.EncodeToString(hashBytes[:])
	backend := &fakeBackend{
		plan: lucy.EnvironmentPlan{Metadata: map[string]string{}},
		lock: lucy.EnvironmentLock{
			LockID:      "lock-1",
			LockHash:    "lockhash",
			GeneratedAt: time.Now().UTC(),
			Packages: []lucy.LockedPackage{
				{ID: "fabric/carpet", Source: "modrinth", Name: "carpet", Version: "1.4.83", Hash: hash, Size: int64(len(content))},
			},
			ProviderMetadata: map[string]string{},
		},
		installContent: content,
	}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)
	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-integrity-ok"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.LucyIntegrityStatus != "ok" {
		t.Errorf("integrity status: got %q, want ok", result.LucyIntegrityStatus)
	}
	if result.Status != "prepared" {
		t.Errorf("status: got %q, want prepared", result.Status)
	}
}

func TestMaterializeEnvironmentWithIntegrityFailRejectsStart(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{
		plan: lucy.EnvironmentPlan{Metadata: map[string]string{}},
		lock: lucy.EnvironmentLock{
			LockID:      "lock-1",
			LockHash:    "lockhash",
			GeneratedAt: time.Now().UTC(),
			Packages: []lucy.LockedPackage{
				{ID: "fabric/carpet", Source: "modrinth", Name: "carpet", Version: "1.4.83", Hash: strings.Repeat("0", 64), Size: 9},
			},
			ProviderMetadata: map[string]string{},
		},
		installContent: []byte("wrong jar"),
	}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)
	_, err = supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-integrity-bad"))
	if err == nil {
		t.Fatal("expected integrity error")
	}
	var integrityErr *agent.EnvironmentIntegrityError
	if !errors.As(err, &integrityErr) {
		t.Fatalf("expected EnvironmentIntegrityError, got %T: %v", err, err)
	}
	if integrityErr.Status != "hash_mismatch" || len(integrityErr.Corrupt) != 1 {
		t.Fatalf("unexpected integrity error: %#v", integrityErr)
	}
	modPath := filepath.Join(root, "sessions", "session-integrity-bad", "mods", "carpet-1.4.83.jar")
	if _, statErr := os.Stat(modPath); statErr != nil {
		t.Fatalf("bad mod file should exist for diagnosis: %v", statErr)
	}
}

func TestMaterializeEnvironmentWithIntegrityMissingFileRejectsStart(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{
		plan: lucy.EnvironmentPlan{Metadata: map[string]string{}},
		lock: lucy.EnvironmentLock{
			LockID:      "lock-1",
			LockHash:    "lockhash",
			GeneratedAt: time.Now().UTC(),
			Packages: []lucy.LockedPackage{
				{ID: "fabric/carpet", Source: "modrinth", Name: "carpet", Version: "1.4.83", Hash: strings.Repeat("1", 64), Size: 1},
			},
			ProviderMetadata: map[string]string{},
		},
		installResult: lucy.InstallPackagesResult{Installed: []lucy.InstalledPackage{}, Failed: []lucy.FailedPackage{}, Status: "ok", TotalSize: 0},
	}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)
	_, err = supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-integrity-missing"))
	if err == nil {
		t.Fatal("expected integrity error")
	}
	var integrityErr *agent.EnvironmentIntegrityError
	if !errors.As(err, &integrityErr) {
		t.Fatalf("expected EnvironmentIntegrityError, got %T: %v", err, err)
	}
	if integrityErr.Status != "missing_files" || len(integrityErr.Missing) != 1 {
		t.Fatalf("unexpected integrity error: %#v", integrityErr)
	}
}

func TestMaterializeEnvironmentWithNoopAdapterIntegrityNotChecked(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-integrity-noop"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.LucyIntegrityStatus != "not_checked" {
		t.Errorf("integrity status: got %q, want not_checked", result.LucyIntegrityStatus)
	}
}

func TestMaterializeEnvironmentWithIntegrityOnLockOnlyNoInstall(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{
		plan: lucy.EnvironmentPlan{Metadata: map[string]string{}},
		lock: lucy.EnvironmentLock{LockID: "lock-1", LockHash: "lockhash", GeneratedAt: time.Now().UTC(), ProviderMetadata: map[string]string{}},
	}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)
	result, err := supervisor.MaterializeEnvironment(context.Background(), testMaterializationRequest("session-integrity-empty-lock"))
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.LucyIntegrityStatus != "ok" {
		t.Errorf("integrity status: got %q, want ok", result.LucyIntegrityStatus)
	}
}

func testMaterializationRequest(sessionID string) agent.EnvironmentMaterializationRequest {
	return agent.EnvironmentMaterializationRequest{
		SessionID:              sessionID,
		EnvironmentID:          "env-117-fabric",
		EnvironmentName:        "1.17 Fabric Carpet",
		MinecraftVersion:       "1.17.1",
		JavaVersion:            "17",
		LoaderType:             "fabric",
		LoaderVersion:          "0.12.0",
		ServerCore:             "carpet",
		MCDRRequired:           true,
		CarpetRequired:         true,
		RuntimeProfileID:       "dummy-process",
		RuntimeProfileRequired: true,
		ActorID:                "alice",
	}
}

type fakeBackend struct {
	caps            lucy.Capabilities
	plan            lucy.EnvironmentPlan
	lock            lucy.EnvironmentLock
	status          lucy.EnvironmentStatus
	installResult   lucy.InstallPackagesResult
	installErr      error
	installContent  []byte
	installCalled   bool
	integrityResult lucy.IntegrityResult
	integrityErr    error
	err             error
}

func (f *fakeBackend) Capabilities(_ context.Context) (lucy.Capabilities, error) {
	return f.caps, f.err
}

func (f *fakeBackend) Plan(_ context.Context, _ lucy.EnvironmentSpec) (lucy.EnvironmentPlan, error) {
	return f.plan, f.err
}

func (f *fakeBackend) Lock(_ context.Context, _ lucy.EnvironmentSpec) (lucy.EnvironmentLock, error) {
	return f.lock, f.err
}

func (f *fakeBackend) Status(_ context.Context, _ lucy.EnvironmentSpec, _ *lucy.EnvironmentLock) (lucy.EnvironmentStatus, error) {
	return f.status, f.err
}

func (f *fakeBackend) Install(_ context.Context, req lucy.InstallPackagesRequest) (lucy.InstallPackagesResult, error) {
	f.installCalled = true
	if f.installErr != nil {
		return lucy.InstallPackagesResult{}, f.installErr
	}
	if f.installResult.Status != "" {
		return f.installResult, f.err
	}
	installed := make([]lucy.InstalledPackage, 0, len(req.Packages))
	var total int64
	for _, pkg := range req.Packages {
		content := f.installContent
		if content == nil {
			content = []byte(pkg.Name)
		}
		path := filepath.Join(req.TargetDir, pkg.Name+"-"+pkg.Version+".jar")
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return lucy.InstallPackagesResult{}, err
		}
		installed = append(installed, lucy.InstalledPackage{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Path: path, Hash: pkg.Hash, Size: int64(len(content))})
		total += int64(len(content))
	}
	return lucy.InstallPackagesResult{Installed: installed, Failed: []lucy.FailedPackage{}, Status: "ok", TotalSize: total}, f.err
}

func (f *fakeBackend) VerifyIntegrity(ctx context.Context, req lucy.IntegrityRequest) (lucy.IntegrityResult, error) {
	if f.integrityErr != nil {
		return lucy.IntegrityResult{}, f.integrityErr
	}
	if f.integrityResult.Status != "" {
		return f.integrityResult, f.err
	}
	return lucy.NewProbeService(req.ModsDir).VerifyIntegrityFromLock(ctx, req.LockPath, req.ModsDir)
}
