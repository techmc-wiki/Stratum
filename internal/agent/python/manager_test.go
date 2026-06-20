package python

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recordedCommand struct {
	Path string
	Args []string
}

func TestBuildVenvResult(t *testing.T) {
	result := BuildVenvResult("s-1", "venv")
	if result.SessionID != "s-1" || result.VenvPath != "venv" {
		t.Fatalf("result=%+v", result)
	}
	if result.PythonExec == "" || result.PipExec == "" || result.MCDRExecutable == "" {
		t.Fatalf("missing executables: %+v", result)
	}
}

func TestMCDRPackageSpec(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{version: "", want: "mcdreforged"},
		{version: "latest", want: "mcdreforged"},
		{version: "2.15.7", want: "mcdreforged==2.15.7"},
	}
	for _, test := range tests {
		if got := MCDRPackageSpec(test.version); got != test.want {
			t.Fatalf("MCDRPackageSpec(%q)=%q want %q", test.version, got, test.want)
		}
	}
}

func TestCreateVenvRunsExpectedCommands(t *testing.T) {
	var commands []recordedCommand
	manager := &Manager{ManagerType: ManagerVenv, Run: func(_ context.Context, path string, args ...string) (string, error) {
		commands = append(commands, recordedCommand{Path: path, Args: append([]string(nil), args...)})
		return "ok", nil
	}}

	result, err := manager.CreateVenv(context.Background(), VenvRequest{
		SessionID: "s-1",
		VenvPath:  t.TempDir() + "/venv",
		Python: Installation{
			ExecutablePath: "py",
			PrefixArgs:     []string{"-3"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "s-1" || result.PythonExec == "" || result.PipExec == "" {
		t.Fatalf("result=%+v", result)
	}
	if len(commands) != 3 {
		t.Fatalf("commands=%+v", commands)
	}
	if commands[0].Path != "py" || !reflect.DeepEqual(commands[0].Args, []string{"-3", "-m", "venv", result.VenvPath}) {
		t.Fatalf("create command=%+v", commands[0])
	}
	if commands[1].Path != result.PythonExec || !reflect.DeepEqual(commands[1].Args, []string{"--version"}) {
		t.Fatalf("python verify command=%+v", commands[1])
	}
	if commands[2].Path != result.PipExec || !reflect.DeepEqual(commands[2].Args, []string{"--version"}) {
		t.Fatalf("pip verify command=%+v", commands[2])
	}
}

func TestNewManagerDefaultsToUV(t *testing.T) {
	manager := NewManager()
	if manager.ManagerType != ManagerUV {
		t.Fatalf("manager type=%q want %q", manager.ManagerType, ManagerUV)
	}
}

func TestCreateVenvValidatesInput(t *testing.T) {
	manager := NewManager()
	_, err := manager.CreateVenv(context.Background(), VenvRequest{})
	if err == nil || !strings.Contains(err.Error(), "session id") {
		t.Fatalf("expected session id error, got %v", err)
	}
}

func TestInstallMCDRBuildsPipCommand(t *testing.T) {
	var command recordedCommand
	manager := &Manager{ManagerType: ManagerVenv, Run: func(_ context.Context, path string, args ...string) (string, error) {
		command = recordedCommand{Path: path, Args: append([]string(nil), args...)}
		return "ok", nil
	}}
	venv := BuildVenvResult("s-1", "venv")
	err := manager.InstallMCDR(context.Background(), InstallMCDRRequest{
		Venv:      venv,
		Version:   "2.15.7",
		IndexURL:  "https://pypi.example/simple",
		ProxyURL:  "http://127.0.0.1:10808",
		ExtraArgs: []string{"--upgrade"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.Path != venv.PipExec {
		t.Fatalf("command path=%q want %q", command.Path, venv.PipExec)
	}
	want := []string{"install", "-i", "https://pypi.example/simple", "--proxy", "http://127.0.0.1:10808", "--upgrade", "mcdreforged==2.15.7"}
	if !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("args=%+v want %+v", command.Args, want)
	}
}

func TestInstallMCDRRequiresPip(t *testing.T) {
	err := (&Manager{ManagerType: ManagerVenv}).InstallMCDR(context.Background(), InstallMCDRRequest{})
	if err == nil || !strings.Contains(err.Error(), "pip") {
		t.Fatalf("expected pip error, got %v", err)
	}
}

func TestVerifyMCDR(t *testing.T) {
	venv := BuildVenvResult("s-1", "venv")
	manager := &Manager{Run: func(_ context.Context, path string, args ...string) (string, error) {
		if path != venv.MCDRExecutable || !reflect.DeepEqual(args, []string{"--version"}) {
			return "", errors.New("unexpected command")
		}
		return "MCDReforged v2.15.7\n", nil
	}}
	version, err := manager.VerifyMCDR(context.Background(), venv)
	if err != nil {
		t.Fatal(err)
	}
	if version != "MCDReforged v2.15.7" {
		t.Fatalf("version=%q", version)
	}
}
