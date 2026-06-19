package mcdr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentjava "github.com/stratummc/stratum/internal/agent/java"
)

func TestPreconditionCheckerReady(t *testing.T) {
	root := t.TempDir()
	serverJar := filepath.Join(root, "server.jar")
	eula := filepath.Join(root, "eula.txt")
	if err := os.WriteFile(serverJar, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eula, []byte("eula=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checker := &PreconditionChecker{
		RunCommand: func(_ context.Context, path string, args ...string) (string, error) {
			if path != "mcdr" || strings.Join(args, " ") != "--version" {
				return "", errors.New("unexpected command")
			}
			return "MCDReforged v2.15.7", nil
		},
		JavaDetector: &agentjava.Detector{
			RunVersion: func(context.Context, string) (string, error) { return `openjdk version "16.0.2"`, nil },
		},
	}

	result := checker.Check(context.Background(), PreconditionRequest{
		SessionID:        "s-1",
		MinecraftVersion: "1.17.1",
		JavaExecutable:   "java16",
		MCDRExecutable:   "mcdr",
		ServerJarPath:    serverJar,
		EULAPath:         eula,
		RequireEULA:      true,
	})
	if !result.Ready {
		t.Fatalf("result=%+v", result)
	}
	for _, check := range result.Checks {
		if check.Status != PreconditionOK {
			t.Fatalf("check=%+v result=%+v", check, result)
		}
	}
}

func TestPreconditionCheckerRejectsOldJava(t *testing.T) {
	checker := &PreconditionChecker{
		RunCommand: func(context.Context, string, ...string) (string, error) { return "MCDReforged v2.15.7", nil },
		JavaDetector: &agentjava.Detector{
			RunVersion: func(context.Context, string) (string, error) { return `openjdk version "1.8.0_402"`, nil },
		},
		Stat:     func(path string) (os.FileInfo, error) { return fakeFileInfo{name: filepath.Base(path)}, nil },
		ReadFile: func(string) ([]byte, error) { return []byte("eula=true\n"), nil },
	}

	result := checker.Check(context.Background(), PreconditionRequest{
		SessionID:        "s-1",
		MinecraftVersion: "1.17.1",
		JavaExecutable:   "java8",
		MCDRExecutable:   "mcdr",
		ServerJarPath:    "server.jar",
		EULAPath:         "eula.txt",
		RequireEULA:      true,
	})
	if result.Ready {
		t.Fatalf("expected not ready: %+v", result)
	}
	check := findCheck(result, "java")
	if check.Status != PreconditionInvalid || !strings.Contains(check.Message, "does not satisfy") {
		t.Fatalf("java check=%+v", check)
	}
}

func TestPreconditionCheckerReportsMissingServerJarAndEULA(t *testing.T) {
	checker := &PreconditionChecker{
		RunCommand: func(context.Context, string, ...string) (string, error) { return "MCDReforged v2.15.7", nil },
		JavaDetector: &agentjava.Detector{
			RunVersion: func(context.Context, string) (string, error) { return `openjdk version "17.0.10"`, nil },
		},
		Stat: func(path string) (os.FileInfo, error) { return nil, os.ErrNotExist },
	}

	result := checker.Check(context.Background(), PreconditionRequest{
		SessionID:        "s-1",
		MinecraftVersion: "1.18.2",
		JavaExecutable:   "java17",
		MCDRExecutable:   "mcdr",
		ServerJarPath:    "server.jar",
		EULAPath:         "eula.txt",
		RequireEULA:      true,
	})
	if result.Ready {
		t.Fatalf("expected not ready: %+v", result)
	}
	if findCheck(result, "server_jar").Status != PreconditionMissing {
		t.Fatalf("server jar check=%+v", findCheck(result, "server_jar"))
	}
	if findCheck(result, "eula").Status != PreconditionMissing {
		t.Fatalf("eula check=%+v", findCheck(result, "eula"))
	}
}

func TestPreconditionCheckerRejectsUnacceptedEULA(t *testing.T) {
	checker := &PreconditionChecker{
		RunCommand: func(context.Context, string, ...string) (string, error) { return "MCDReforged v2.15.7", nil },
		JavaDetector: &agentjava.Detector{
			RunVersion: func(context.Context, string) (string, error) { return `openjdk version "17.0.10"`, nil },
		},
		Stat:     func(path string) (os.FileInfo, error) { return fakeFileInfo{name: filepath.Base(path)}, nil },
		ReadFile: func(string) ([]byte, error) { return []byte("eula=false\n"), nil },
	}

	result := checker.Check(context.Background(), PreconditionRequest{
		SessionID:        "s-1",
		MinecraftVersion: "1.18.2",
		JavaExecutable:   "java17",
		MCDRExecutable:   "mcdr",
		ServerJarPath:    "server.jar",
		EULAPath:         "eula.txt",
		RequireEULA:      true,
	})
	if result.Ready {
		t.Fatalf("expected not ready: %+v", result)
	}
	check := findCheck(result, "eula")
	if check.Status != PreconditionInvalid || !strings.Contains(check.Message, "eula=true") {
		t.Fatalf("eula check=%+v", check)
	}
}

func TestPreconditionCheckerCanSkipEULA(t *testing.T) {
	checker := &PreconditionChecker{
		RunCommand: func(context.Context, string, ...string) (string, error) { return "MCDReforged v2.15.7", nil },
		JavaDetector: &agentjava.Detector{
			RunVersion: func(context.Context, string) (string, error) { return `openjdk version "17.0.10"`, nil },
		},
		Stat: func(path string) (os.FileInfo, error) { return fakeFileInfo{name: filepath.Base(path)}, nil },
	}

	result := checker.Check(context.Background(), PreconditionRequest{
		SessionID:        "s-1",
		MinecraftVersion: "1.18.2",
		JavaExecutable:   "java17",
		MCDRExecutable:   "mcdr",
		ServerJarPath:    "server.jar",
	})
	if !result.Ready {
		t.Fatalf("expected ready with skipped EULA: %+v", result)
	}
	if findCheck(result, "eula").Status != PreconditionSkipped {
		t.Fatalf("eula check=%+v", findCheck(result, "eula"))
	}
}

func findCheck(result PreconditionResult, name string) PreconditionCheck {
	for _, check := range result.Checks {
		if check.Name == name {
			return check
		}
	}
	return PreconditionCheck{}
}

type fakeFileInfo struct {
	name  string
	isDir bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 1 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0o644 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }
