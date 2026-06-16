package mcdrbridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent/process"
)

func testRuntimeRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "runtime")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatalf("create runtime root: %v", err)
	}
	return root
}

func testBuildRequest(sessionID string) BuildLaunchPlanRequest {
	return BuildLaunchPlanRequest{
		SessionID:        sessionID,
		EnvironmentID:    "env-1",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		LoaderVersion:    "0.14.0",
		ServerCore:       "fabric-server",
		RuntimeProfileID: "default-terminal",
		MCDRRequired:     true,
		CarpetRequired:   false,
		Metadata: map[string]string{
			"source": "test",
		},
	}
}

func TestBuildLaunchPlanCreatesManifest(t *testing.T) {
	root := testRuntimeRoot(t)
	bridge := NewBridge(root)
	req := testBuildRequest("session-1")

	plan, err := bridge.BuildLaunchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildLaunchPlan: %v", err)
	}

	if plan.SessionID != "session-1" {
		t.Errorf("SessionID = %q, want %q", plan.SessionID, "session-1")
	}
	if plan.Status != StatusPlanned {
		t.Errorf("Status = %q, want %q", plan.Status, StatusPlanned)
	}
	if plan.Notes == "" {
		t.Error("Notes should not be empty")
	}

	sessionLayout, err := process.NewSessionRuntimeLayout(root, "session-1")
	if err != nil {
		t.Fatalf("session layout: %v", err)
	}
	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatalf("MCDR layout: %v", err)
	}

	manifestPath := filepath.Join(mcdrLayout.MCDRRoot, launchPlanManifestName)
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatalf("manifest was not written at %s", manifestPath)
	}

	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var persisted LaunchPlan
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if persisted.SessionID != "session-1" {
		t.Errorf("persisted SessionID = %q", persisted.SessionID)
	}
	if persisted.Status != StatusPlanned {
		t.Errorf("persisted Status = %q", persisted.Status)
	}
	if persisted.Command.Executable != "python" {
		t.Errorf("persisted Command.Executable = %q, want python", persisted.Command.Executable)
	}
}

func TestBuildLaunchPlanIsIdempotent(t *testing.T) {
	root := testRuntimeRoot(t)
	bridge := NewBridge(root)
	req := testBuildRequest("session-2")

	plan1, err := bridge.BuildLaunchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("first BuildLaunchPlan: %v", err)
	}

	plan2, err := bridge.BuildLaunchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("second BuildLaunchPlan: %v", err)
	}

	if plan1.SessionID != plan2.SessionID {
		t.Errorf("SessionID mismatch: %q vs %q", plan1.SessionID, plan2.SessionID)
	}
	if plan1.Status != plan2.Status {
		t.Errorf("Status mismatch: %q vs %q", plan1.Status, plan2.Status)
	}
	if plan1.MCDRRoot != plan2.MCDRRoot {
		t.Errorf("MCDRRoot mismatch: %q vs %q", plan1.MCDRRoot, plan2.MCDRRoot)
	}
	if plan1.Command.Executable != plan2.Command.Executable {
		t.Errorf("Command.Executable mismatch: %q vs %q", plan1.Command.Executable, plan2.Command.Executable)
	}
}

func TestInspectLaunchPlanMissing(t *testing.T) {
	root := testRuntimeRoot(t)
	bridge := NewBridge(root)

	// Prepare session layout so InspectLaunchPlan can build paths.
	sessionLayout, err := process.NewSessionRuntimeLayout(root, "session-missing")
	if err != nil {
		t.Fatalf("session layout: %v", err)
	}
	if err := sessionLayout.Create(); err != nil {
		t.Fatalf("create session layout: %v", err)
	}
	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatalf("MCDR layout: %v", err)
	}
	if err := mcdrLayout.Create(); err != nil {
		t.Fatalf("create MCDR layout: %v", err)
	}

	status, err := bridge.InspectLaunchPlan(context.Background(), "session-missing")
	if err != nil {
		t.Fatalf("InspectLaunchPlan: %v", err)
	}
	if status.Exists {
		t.Error("Exists should be false when manifest is missing")
	}
	if status.Valid {
		t.Error("Valid should be false when manifest is missing")
	}
	if status.Status != "missing" {
		t.Errorf("Status = %q, want missing", status.Status)
	}
}

func TestInspectLaunchPlanValid(t *testing.T) {
	root := testRuntimeRoot(t)
	bridge := NewBridge(root)
	req := testBuildRequest("session-3")

	_, err := bridge.BuildLaunchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildLaunchPlan: %v", err)
	}

	status, err := bridge.InspectLaunchPlan(context.Background(), "session-3")
	if err != nil {
		t.Fatalf("InspectLaunchPlan: %v", err)
	}
	if !status.Exists {
		t.Error("Exists should be true")
	}
	if !status.Valid {
		t.Error("Valid should be true for a freshly generated plan")
	}
	if status.Status != StatusPlanned {
		t.Errorf("Status = %q, want %q", status.Status, StatusPlanned)
	}
}

func TestBuildLaunchPlanRejectsUnsafeSessionID(t *testing.T) {
	root := testRuntimeRoot(t)
	bridge := NewBridge(root)

	unsafeIDs := []string{"", ".", "..", "../escape", "session with spaces", "session<script>"}
	for _, id := range unsafeIDs {
		req := testBuildRequest(id)
		_, err := bridge.BuildLaunchPlan(context.Background(), req)
		if err == nil {
			t.Errorf("expected error for unsafe session ID %q", id)
		}
	}
}

func TestLaunchPlanRejectsTraversalPaths(t *testing.T) {
	// ValidateLaunchPlan should reject paths that escape via traversal.
	bridge := NewBridge("dummy-root")

	plan := LaunchPlan{
		SessionID: "session-1",
		Status:    StatusPlanned,
		MCDRRoot:  "../escape",
		Command: LaunchCommand{
			Executable: "python",
			Argv:       []string{"-m", "mcdreforged"},
			WorkingDir: "mcdr",
		},
		Stop: StopConfig{
			Strategy: StopStrategyStdin,
		},
		Preconditions: Preconditions{
			PythonAvailable:  PreconditionNotChecked,
			MCDRInstalled:    PreconditionNotChecked,
			ServerJarPresent: PreconditionNotChecked,
			EULAPresent:      PreconditionNotChecked,
		},
		PlannedAt: time.Now().UTC(),
	}

	err := bridge.ValidateLaunchPlan(context.Background(), plan)
	if err == nil {
		t.Error("expected error for plan with traversal path")
	}
	t.Logf("traversal rejection: %v", err)
}

func TestLaunchPlanRejectsAbsolutePaths(t *testing.T) {
	bridge := NewBridge("dummy-root")

	plan := LaunchPlan{
		SessionID: "session-1",
		Status:    StatusPlanned,
		MCDRRoot:  "mcdr",
		ConfigDir: "/etc/mcdr",
		Command: LaunchCommand{
			Executable: "python",
			Argv:       []string{"-m", "mcdreforged"},
			WorkingDir: "mcdr",
		},
		Stop: StopConfig{
			Strategy: StopStrategyStdin,
		},
		Preconditions: Preconditions{
			PythonAvailable:  PreconditionNotChecked,
			MCDRInstalled:    PreconditionNotChecked,
			ServerJarPresent: PreconditionNotChecked,
			EULAPresent:      PreconditionNotChecked,
		},
		PlannedAt: time.Now().UTC(),
	}

	err := bridge.ValidateLaunchPlan(context.Background(), plan)
	if err == nil {
		t.Error("expected error for plan with absolute path")
	}
	t.Logf("absolute path rejection: %v", err)
}

func TestLaunchPlanCommandIsArgvBased(t *testing.T) {
	bridge := NewBridge("dummy-root")

	plan := LaunchPlan{
		SessionID:                   "session-1",
		Status:                      StatusPlanned,
		MCDRRoot:                    "work/mcdr",
		ConfigDir:                   "work/mcdr/config",
		PluginsDir:                  "work/mcdr/plugins",
		ServerDir:                   "work/mcdr/server",
		LogsDir:                     "work/mcdr/logs",
		ServerWorkDir:               "work/mcdr/server",
		PlannedConfigPath:           "work/mcdr/config/config.yml",
		PlannedServerPropertiesPath: "work/mcdr/server/server.properties",
		PlannedEULAPath:             "work/mcdr/server/eula.txt",
		Command: LaunchCommand{
			Executable: "python",
			Argv:       []string{"-m", "mcdreforged"},
			WorkingDir: "work/mcdr",
		},
		Stop: StopConfig{
			Strategy: StopStrategyStdin,
		},
		Preconditions: Preconditions{
			PythonAvailable:  PreconditionNotChecked,
			MCDRInstalled:    PreconditionNotChecked,
			ServerJarPresent: PreconditionNotChecked,
			EULAPresent:      PreconditionNotChecked,
		},
		PlannedAt: time.Now().UTC(),
	}

	err := bridge.ValidateLaunchPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("ValidateLaunchPlan should accept valid argv-based plan: %v", err)
	}

	if len(plan.Command.Argv) == 0 {
		t.Error("plan should have argv entries")
	}
	if plan.Command.Executable != "python" {
		t.Errorf("expected python executable, got %q", plan.Command.Executable)
	}
}

func TestBuildLaunchPlanDoesNotStartProcesses(t *testing.T) {
	root := testRuntimeRoot(t)
	bridge := NewBridge(root)
	req := testBuildRequest("session-no-start")

	_, err := bridge.BuildLaunchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildLaunchPlan: %v", err)
	}

	// Verify no process-related files exist: no server.jar, no eula.txt,
	// no Python scripts were executed.
	sessionLayout, err := process.NewSessionRuntimeLayout(root, "session-no-start")
	if err != nil {
		t.Fatalf("session layout: %v", err)
	}

	// Only the manifest should exist in the MCDR root, nothing else that
	// indicates process execution.
	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatalf("MCDR layout: %v", err)
	}

	entries, err := os.ReadDir(mcdrLayout.MCDRRoot)
	if err != nil {
		t.Fatalf("read MCDR root: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == launchPlanManifestName || name == "mcdr-layout.json" || name == "mcdr-config-stub.json" {
			continue
		}
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(name) == ".json" {
			continue
		}
		t.Errorf("unexpected file %s in MCDR root; no processes should have been started", name)
	}
}

func TestInspectLaunchPlanReadOnly(t *testing.T) {
	root := testRuntimeRoot(t)
	bridge := NewBridge(root)
	req := testBuildRequest("session-readonly")

	_, err := bridge.BuildLaunchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildLaunchPlan: %v", err)
	}

	sessionLayout, err := process.NewSessionRuntimeLayout(root, "session-readonly")
	if err != nil {
		t.Fatalf("session layout: %v", err)
	}
	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatalf("MCDR layout: %v", err)
	}
	manifestPath := filepath.Join(mcdrLayout.MCDRRoot, launchPlanManifestName)

	origStat, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}

	_, err = bridge.InspectLaunchPlan(context.Background(), "session-readonly")
	if err != nil {
		t.Fatalf("InspectLaunchPlan: %v", err)
	}

	afterStat, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatalf("stat manifest after inspect: %v", err)
	}

	if !origStat.ModTime().Equal(afterStat.ModTime()) {
		t.Error("InspectLaunchPlan modified the manifest file")
	}
}

func TestBuildLaunchPlanRejectsInvalidStatus(t *testing.T) {
	bridge := NewBridge("dummy-root")

	plan := LaunchPlan{
		SessionID:                   "session-1",
		Status:                      "running",
		MCDRRoot:                    "work/mcdr",
		ConfigDir:                   "work/mcdr/config",
		PluginsDir:                  "work/mcdr/plugins",
		ServerDir:                   "work/mcdr/server",
		LogsDir:                     "work/mcdr/logs",
		ServerWorkDir:               "work/mcdr/server",
		PlannedConfigPath:           "work/mcdr/config/config.yml",
		PlannedServerPropertiesPath: "work/mcdr/server/server.properties",
		PlannedEULAPath:             "work/mcdr/server/eula.txt",
		Command: LaunchCommand{
			Executable: "python",
			Argv:       []string{"-m", "mcdreforged"},
			WorkingDir: "work/mcdr",
		},
		Stop: StopConfig{
			Strategy: StopStrategyStdin,
		},
		Preconditions: Preconditions{
			PythonAvailable:  PreconditionNotChecked,
			MCDRInstalled:    PreconditionNotChecked,
			ServerJarPresent: PreconditionNotChecked,
			EULAPresent:      PreconditionNotChecked,
		},
		PlannedAt: time.Now().UTC(),
	}

	err := bridge.ValidateLaunchPlan(context.Background(), plan)
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestBuildLaunchPlansMetadata(t *testing.T) {
	root := testRuntimeRoot(t)
	bridge := NewBridge(root)
	req := testBuildRequest("session-meta")

	plan, err := bridge.BuildLaunchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildLaunchPlan: %v", err)
	}

	if plan.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}
	if plan.Metadata["source"] != "test" {
		t.Errorf("Metadata[source] = %q, want test", plan.Metadata["source"])
	}
	if plan.EnvironmentID != "env-1" {
		t.Errorf("EnvironmentID = %q, want env-1", plan.EnvironmentID)
	}
	if plan.MinecraftVersion != "1.17.1" {
		t.Errorf("MinecraftVersion = %q, want 1.17.1", plan.MinecraftVersion)
	}
	if plan.LoaderType != "fabric" {
		t.Errorf("LoaderType = %q, want fabric", plan.LoaderType)
	}
}

func TestLaunchPlanPreconditionsDefault(t *testing.T) {
	root := testRuntimeRoot(t)
	bridge := NewBridge(root)
	req := testBuildRequest("session-pc")

	plan, err := bridge.BuildLaunchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildLaunchPlan: %v", err)
	}

	if plan.Preconditions.PythonAvailable != PreconditionNotChecked {
		t.Errorf("PythonAvailable = %q, want not_checked", plan.Preconditions.PythonAvailable)
	}
	if plan.Preconditions.MCDRInstalled != PreconditionNotChecked {
		t.Errorf("MCDRInstalled = %q, want not_checked", plan.Preconditions.MCDRInstalled)
	}
	if plan.Preconditions.ServerJarPresent != PreconditionNotChecked {
		t.Errorf("ServerJarPresent = %q, want not_checked", plan.Preconditions.ServerJarPresent)
	}
	if plan.Preconditions.EULAPresent != PreconditionNotChecked {
		t.Errorf("EULAPresent = %q, want not_checked", plan.Preconditions.EULAPresent)
	}
}
