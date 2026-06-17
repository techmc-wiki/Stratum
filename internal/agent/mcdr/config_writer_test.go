package mcdr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stratummc/stratum/internal/agent/process"
)

func TestWriteRuntimeConfigWritesConfigYML(t *testing.T) {
	layout := testMCDRRuntimeLayout(t)
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	path, err := WriteRuntimeConfig(layout, RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(layout.MCDRConfigDir, configYMLName) {
		t.Fatalf("path=%q", path)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(payload)
	for _, want := range []string{
		"working_directory: " + quoteYAMLString(layout.MCDRServerDir),
		"  - " + quoteYAMLString(layout.MCDRPluginsDir),
		"file: " + quoteYAMLString(filepath.Join(layout.MCDRLogsDir, "mcdr.log")),
		"config_directory: " + quoteYAMLString(layout.MCDRConfigDir),
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("config.yml missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(strings.ToLower(content), "token") || strings.Contains(strings.ToLower(content), "secret") {
		t.Fatalf("config.yml should not contain secret-like fields:\n%s", content)
	}
}

func TestWriteRuntimeConfigRejectsHostPathsOutsideLayout(t *testing.T) {
	layout := testMCDRRuntimeLayout(t)
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(escape, 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := NewRuntimeConfig(layout)
	cfg.PluginDir = escape
	if _, err := WriteRuntimeConfig(layout, cfg); err == nil || !strings.Contains(err.Error(), "Agent MCDR runtime layout") {
		t.Fatalf("expected outside path rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(escape, configYMLName)); !os.IsNotExist(err) {
		t.Fatalf("writer should not create escaped config: %v", err)
	}
}

func TestWriteRuntimeConfigRejectsEscapedConfigDirectory(t *testing.T) {
	layout := testMCDRRuntimeLayout(t)
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	escaped := filepath.Join(t.TempDir(), "escaped-config")
	if err := os.MkdirAll(escaped, 0o750); err != nil {
		t.Fatal(err)
	}
	layout.MCDRConfigDir = escaped
	if _, err := WriteRuntimeConfig(layout, RuntimeConfig{}); err == nil || !strings.Contains(err.Error(), "escapes MCDR runtime layout") {
		t.Fatalf("expected escaped config dir rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(escaped, configYMLName)); !os.IsNotExist(err) {
		t.Fatalf("writer should not create config outside runtime layout: %v", err)
	}
}

func testMCDRRuntimeLayout(t *testing.T) process.MCDRRuntimeLayout {
	t.Helper()
	sessionLayout, err := process.NewSessionRuntimeLayout(t.TempDir(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	return layout
}
