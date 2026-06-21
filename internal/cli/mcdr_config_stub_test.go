package cli

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/agent/mcdr"
	agentprocess "github.com/stratummc/stratum/internal/agent/process"
)

func TestSessionsMCDRConfigStubInspectMissing(t *testing.T) {
	runtimeRoot := t.TempDir()
	processAgent, err := local.NewProcessAgentWithRegistryAndRoot("test-agent", nil, runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httptransport.NewServer(processAgent, "token", nil).Handler())
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--agent-url", server.URL, "--agent-token", "token", "sessions", "mcdr-config-stub", "inspect", "--id", "session-1"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for missing manifest")
	}
	output := stdout.String()
	if !strings.Contains(output, "Exists:    false") {
		t.Fatalf("expected Exists: false, got: %s", output)
	}
	if !strings.Contains(output, "Status:    missing") {
		t.Fatalf("expected Status: missing, got: %s", output)
	}
}

func TestSessionsMCDRConfigStubInspectValid(t *testing.T) {
	runtimeRoot := t.TempDir()
	sessionLayout, err := agentprocess.NewSessionRuntimeLayout(runtimeRoot, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.MCDRRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	stub, err := mcdr.NewConfigStub(layout, agent.EnvironmentMaterializationResult{SessionID: "session-1", EnvironmentID: "env-1", MinecraftVersion: "1.17.1", JavaVersion: "17", MCDRRequired: true, Status: "prepared"}, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mcdr.WriteConfigStubManifest(layout, stub); err != nil {
		t.Fatal(err)
	}
	processAgent, err := local.NewProcessAgentWithRegistryAndRoot("test-agent", nil, runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httptransport.NewServer(processAgent, "token", nil).Handler())
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--agent-url", server.URL, "--agent-token", "token", "sessions", "mcdr-config-stub", "inspect", "--id", "session-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit for valid manifest, got: %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "Exists:    true") {
		t.Fatalf("expected Exists: true, got: %s", output)
	}
	if !strings.Contains(output, "Valid:     true") {
		t.Fatalf("expected Valid: true, got: %s", output)
	}
	if !strings.Contains(output, "Status:    planned") {
		t.Fatalf("expected Status: planned, got: %s", output)
	}
	if !strings.Contains(output, "Config:") {
		t.Fatalf("expected Config path, got: %s", output)
	}
}

func TestSessionsMCDRConfigStubInspectMalformed(t *testing.T) {
	runtimeRoot := t.TempDir()
	sessionLayout, err := agentprocess.NewSessionRuntimeLayout(runtimeRoot, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.MCDRRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(layout.MCDRRoot, "mcdr-config-stub.json")
	if err := os.WriteFile(manifestPath, []byte("{invalid}"), 0o640); err != nil {
		t.Fatal(err)
	}
	processAgent, err := local.NewProcessAgentWithRegistryAndRoot("test-agent", nil, runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httptransport.NewServer(processAgent, "token", nil).Handler())
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--agent-url", server.URL, "--agent-token", "token", "sessions", "mcdr-config-stub", "inspect", "--id", "session-1"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for malformed manifest")
	}
	output := stdout.String()
	if !strings.Contains(output, "Exists:    true") {
		t.Fatalf("expected Exists: true, got: %s", output)
	}
	if !strings.Contains(output, "Valid:     false") {
		t.Fatalf("expected Valid: false, got: %s", output)
	}
	if !strings.Contains(output, "Issues:") {
		t.Fatalf("expected Issues, got: %s", output)
	}
}

func TestSessionsMCDRConfigStubInspectReadOnly(t *testing.T) {
	runtimeRoot := t.TempDir()
	sessionLayout, err := agentprocess.NewSessionRuntimeLayout(runtimeRoot, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.MCDRRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	stub, err := mcdr.NewConfigStub(layout, agent.EnvironmentMaterializationResult{SessionID: "session-1", EnvironmentID: "env-1", MinecraftVersion: "1.17.1", JavaVersion: "17", MCDRRequired: true, Status: "prepared"}, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mcdr.WriteConfigStubManifest(layout, stub); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(layout.MCDRRoot, "mcdr-config-stub.json")
	before, _ := os.ReadFile(manifestPath)
	processAgent, err := local.NewProcessAgentWithRegistryAndRoot("test-agent", nil, runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httptransport.NewServer(processAgent, "token", nil).Handler())
	defer server.Close()
	var stdout, stderr bytes.Buffer
	_ = Run([]string{"--agent-url", server.URL, "--agent-token", "token", "sessions", "mcdr-config-stub", "inspect", "--id", "session-1"}, &stdout, &stderr)
	after, _ := os.ReadFile(manifestPath)
	if string(before) != string(after) {
		t.Fatal("inspection modified manifest")
	}
	configYMLPath := filepath.Join(layout.MCDRRoot, "config", "config.yml")
	if _, err := os.Stat(configYMLPath); err == nil {
		t.Fatal("inspection created config.yml")
	}
	serverPropertiesPath := filepath.Join(layout.MCDRRoot, "server", "server.properties")
	if _, err := os.Stat(serverPropertiesPath); err == nil {
		t.Fatal("inspection created server.properties")
	}
	eulaPath := filepath.Join(layout.MCDRRoot, "server", "eula.txt")
	if _, err := os.Stat(eulaPath); err == nil {
		t.Fatal("inspection created eula.txt")
	}
}

func TestSessionsMCDRConfigStubRequiresID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--agent-local", "sessions", "mcdr-config-stub", "inspect"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit when --id missing")
	}
	if !strings.Contains(stderr.String(), "--id is required") {
		t.Fatalf("expected --id error, got: %s", stderr.String())
	}
}
