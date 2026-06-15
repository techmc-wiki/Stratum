package mcdr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectConfigStubManifestMissing(t *testing.T) {
	_, _, layout := testConfigStub(t)
	result := InspectConfigStubManifest(layout)
	if result.Exists {
		t.Fatal("expected Exists=false for missing manifest")
	}
	if result.Valid {
		t.Fatal("expected Valid=false for missing manifest")
	}
	if result.Status != "missing" {
		t.Fatalf("status = %q, want missing", result.Status)
	}
}

func TestInspectConfigStubManifestValid(t *testing.T) {
	stub, _, layout := testConfigStub(t)
	if err := os.MkdirAll(layout.MCDRRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteConfigStubManifest(layout, stub); err != nil {
		t.Fatal(err)
	}
	result := InspectConfigStubManifest(layout)
	if !result.Exists {
		t.Fatal("expected Exists=true")
	}
	if !result.Valid {
		t.Fatalf("expected Valid=true, issues: %v", result.Issues)
	}
	if result.Status != "planned" {
		t.Fatalf("status = %q, want planned", result.Status)
	}
	if result.PlannedConfigYMLPath == "" {
		t.Fatal("PlannedConfigYMLPath empty")
	}
}

func TestInspectConfigStubManifestMalformedJSON(t *testing.T) {
	_, _, layout := testConfigStub(t)
	if err := os.MkdirAll(layout.MCDRRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(layout.MCDRRoot, configStubManifestName)
	if err := os.WriteFile(manifestPath, []byte("{invalid json}"), 0o640); err != nil {
		t.Fatal(err)
	}
	result := InspectConfigStubManifest(layout)
	if !result.Exists {
		t.Fatal("expected Exists=true")
	}
	if result.Valid {
		t.Fatal("expected Valid=false for malformed JSON")
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues for malformed JSON")
	}
}

func TestInspectConfigStubManifestUnsafePath(t *testing.T) {
	stub, _, layout := testConfigStub(t)
	if err := os.MkdirAll(layout.MCDRRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteConfigStubManifest(layout, stub); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(layout.MCDRRoot, configStubManifestName)
	var manifest ConfigStubManifest
	payload, _ := os.ReadFile(manifestPath)
	_ = json.Unmarshal(payload, &manifest)
	manifest.MCDRRoot = "../escape"
	modified, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, modified, 0o640); err != nil {
		t.Fatal(err)
	}
	result := InspectConfigStubManifest(layout)
	if result.Valid {
		t.Fatal("expected Valid=false for unsafe path")
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues for unsafe path")
	}
}

func TestInspectConfigStubManifestReadOnly(t *testing.T) {
	stub, _, layout := testConfigStub(t)
	if err := os.MkdirAll(layout.MCDRRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteConfigStubManifest(layout, stub); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(layout.MCDRRoot, configStubManifestName)
	before, _ := os.ReadFile(manifestPath)
	_ = InspectConfigStubManifest(layout)
	after, _ := os.ReadFile(manifestPath)
	if string(before) != string(after) {
		t.Fatal("inspection modified manifest")
	}
}
