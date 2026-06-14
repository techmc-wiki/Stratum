package mcdr

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/process"
)

func TestConfigStubValidates(t *testing.T) {
	stub, _, _ := testConfigStub(t)
	if err := stub.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestConfigStubRejectsUnsafeSessionID(t *testing.T) {
	stub, _, _ := testConfigStub(t)
	stub.SessionID = "../escape"
	if err := stub.Validate(); err == nil {
		t.Fatal("unsafe session id should fail")
	}
}

func TestConfigStubRejectsUnsafePaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "traversal", path: "work/mcdr/../escape"},
		{name: "absolute", path: "/work/mcdr/config"},
		{name: "windows absolute", path: `C:\work\mcdr\config`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub, _, _ := testConfigStub(t)
			stub.ConfigDir = test.path
			if err := stub.Validate(); err == nil {
				t.Fatalf("path %q should fail", test.path)
			}
		})
	}
}

func TestNewConfigStubBuildsPathsWithoutSideEffects(t *testing.T) {
	stub, root, layout := testConfigStub(t)

	if stub.MCDRRoot != "work/mcdr" || stub.ConfigFilePath != "work/mcdr/config/config.yml" {
		t.Fatalf("unexpected config paths: %+v", stub)
	}
	if stub.ServerPropertiesPath != "work/mcdr/server/server.properties" || stub.EULAPath != "work/mcdr/server/eula.txt" {
		t.Fatalf("unexpected server paths: %+v", stub)
	}
	if _, err := os.Stat(layout.MCDRRoot); !os.IsNotExist(err) {
		t.Fatalf("builder created MCDR root: %v", err)
	}
	for _, file := range []string{
		filepath.Join(layout.MCDRConfigDir, "config.yml"),
		filepath.Join(layout.MCDRServerDir, "server.properties"),
		filepath.Join(layout.MCDRServerDir, "eula.txt"),
	} {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("builder wrote %q: %v", file, err)
		}
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("builder should not create files or start a runtime: entries=%v err=%v", entries, err)
	}
}

func TestNewConfigStubRejectsEscapedLayout(t *testing.T) {
	root := t.TempDir()
	sessionLayout, err := process.NewSessionRuntimeLayout(root, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	layout.MCDRConfigDir = filepath.Join(root, "escape")
	_, err = NewConfigStub(layout, testMaterialization(), time.Now())
	if err == nil {
		t.Fatal("escaped layout should fail")
	}
}

func testConfigStub(t *testing.T) (ConfigStub, string, process.MCDRRuntimeLayout) {
	t.Helper()
	root := t.TempDir()
	sessionLayout, err := process.NewSessionRuntimeLayout(root, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	stub, err := NewConfigStub(layout, testMaterialization(), time.Date(2026, 6, 15, 12, 0, 0, 0, time.FixedZone("test", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	return stub, root, layout
}

func testMaterialization() agent.EnvironmentMaterializationResult {
	return agent.EnvironmentMaterializationResult{
		SessionID:        "session-1",
		EnvironmentID:    "env-1",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		MCDRRequired:     true,
		Status:           "prepared",
		Metadata:         map[string]string{"manifestPath": "config/environment-materialization.json"},
	}
}
