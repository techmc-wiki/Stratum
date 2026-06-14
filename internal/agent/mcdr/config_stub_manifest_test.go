package mcdr

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stratummc/stratum/internal/agent/process"
)

func TestSerializeConfigStub(t *testing.T) {
	stub, _, _ := testConfigStub(t)
	payload, err := SerializeConfigStub(stub)
	if err != nil {
		t.Fatalf("SerializeConfigStub: %v", err)
	}
	var manifest ConfigStubManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("serialized manifest is not valid JSON: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["planned_config_yml_path"]; !ok {
		t.Fatal("serialized manifest is missing planned_config_yml_path")
	}
	if _, ok := fields["config_file_path"]; ok {
		t.Fatal("serialized manifest exposed ambiguous config_file_path")
	}
	if manifest.Status != ConfigStubStatusPlanned {
		t.Fatalf("status=%q", manifest.Status)
	}
	if manifest.PlannedConfigYMLPath != "work/mcdr/config/config.yml" || manifest.PlannedServerPropertiesPath != "work/mcdr/server/server.properties" || manifest.PlannedEULAPath != "work/mcdr/server/eula.txt" {
		t.Fatalf("unexpected planning paths: %+v", manifest)
	}
	for _, clarification := range []string{"not MCDR config.yml or Minecraft server.properties", "does not install MCDR", "no MCDR or Minecraft process was started"} {
		if !strings.Contains(manifest.Notes, clarification) {
			t.Fatalf("notes %q do not contain %q", manifest.Notes, clarification)
		}
	}
}

func TestWriteConfigStubManifest(t *testing.T) {
	stub, _, layout := testConfigStub(t)
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	manifestPath, err := WriteConfigStubManifest(layout, stub)
	if err != nil {
		t.Fatalf("WriteConfigStubManifest: %v", err)
	}
	if manifestPath != filepath.Join(layout.MCDRRoot, configStubManifestName) || !filesystemPathWithin(layout.MCDRRoot, manifestPath) {
		t.Fatalf("unsafe manifest path %q", manifestPath)
	}
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest ConfigStubManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("written manifest is not valid JSON: %v", err)
	}
	if manifest.SessionID != stub.SessionID || manifest.MCDRRoot != stub.MCDRRoot || manifest.Status != ConfigStubStatusPlanned {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	assertNoRuntimeConfigFiles(t, layout)
}

func TestWriteConfigStubManifestIsIdempotent(t *testing.T) {
	stub, _, layout := testConfigStub(t)
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	manifestPath, err := WriteConfigStubManifest(layout, stub)
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteConfigStubManifest(layout, stub); err != nil {
		t.Fatalf("repeated write: %v", err)
	}
	second, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("repeated write changed manifest content")
	}
	assertNoRuntimeConfigFiles(t, layout)
}

func TestWriteConfigStubManifestRejectsUnsafeInput(t *testing.T) {
	stub, _, layout := testConfigStub(t)
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*ConfigStub)
	}{
		{name: "unsafe session id", mutate: func(stub *ConfigStub) { stub.SessionID = "../escape" }},
		{name: "path traversal", mutate: func(stub *ConfigStub) { stub.ConfigDir = "work/mcdr/../escape" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := stub
			test.mutate(&invalid)
			if _, err := WriteConfigStubManifest(layout, invalid); err == nil {
				t.Fatal("unsafe stub should fail")
			}
		})
	}
	assertNoRuntimeConfigFiles(t, layout)
}

func TestWriteConfigStubManifestRejectsEscapedLayout(t *testing.T) {
	stub, root, layout := testConfigStub(t)
	escapedRoot := filepath.Join(root, "escaped-mcdr")
	if err := os.MkdirAll(escapedRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	layout.MCDRRoot = escapedRoot
	if _, err := WriteConfigStubManifest(layout, stub); err == nil {
		t.Fatal("escaped MCDR layout should fail")
	}
	if _, err := os.Stat(filepath.Join(escapedRoot, configStubManifestName)); !os.IsNotExist(err) {
		t.Fatalf("writer created manifest outside canonical MCDR root: %v", err)
	}
}

func assertNoRuntimeConfigFiles(t *testing.T, layout process.MCDRRuntimeLayout) {
	t.Helper()
	for _, file := range []string{
		filepath.Join(layout.MCDRConfigDir, "config.yml"),
		filepath.Join(layout.MCDRServerDir, "server.properties"),
		filepath.Join(layout.MCDRServerDir, "eula.txt"),
	} {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("manifest writer created %q: %v", file, err)
		}
	}
}
