package process

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

func TestSupervisorStartStopRestartAndLogs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	started, err := supervisor.StartProcess(ctx, "session-1", runtimeprofile.DummyProcess())
	if err != nil || started.Status != StatusRunning || !supervisor.IsRunning("session-1") || started.WorkDir != filepath.Join(root, "sessions", "session-1", "work") {
		t.Fatalf("started=%+v err=%v", started, err)
	}
	for _, path := range []string{started.SessionRoot, started.WorkDir, started.LogsDir, filepath.Join(root, "sessions", "session-1", "config"), filepath.Join(root, "sessions", "session-1", "artifacts"), filepath.Join(root, "sessions", "session-1", "checkpoints"), filepath.Join(root, "sessions", "session-1", "tmp")} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("runtime directory %q info=%+v err=%v", path, info, err)
		}
	}
	logs := supervisor.CollectLogs("session-1", 80)
	if len(logs) == 0 || !strings.Contains(logs[len(logs)-1], "runtime running") {
		t.Fatalf("logs=%v", logs)
	}
	stopped, err := supervisor.StopProcess(ctx, "session-1")
	if err != nil || stopped.Status != StatusStopped || supervisor.IsRunning("session-1") {
		t.Fatalf("stopped=%+v err=%v", stopped, err)
	}
	restarted, err := supervisor.RestartProcess(ctx, "session-1", runtimeprofile.DummyProcess())
	if err != nil || restarted.Status != StatusRunning || restarted.ProcessID == started.ProcessID {
		t.Fatalf("restarted=%+v err=%v", restarted, err)
	}
	if !containsLog(supervisor.CollectLogs("session-1", 0), "restart boundary") {
		t.Fatal("restart boundary missing")
	}
}

func TestTerminalExecutorStdinStopCapturesLogsAndRestarts(t *testing.T) {
	supervisor, profile := terminalTestSupervisor(t, "stdin")
	started, err := supervisor.StartProcess(context.Background(), "terminal-stdin", profile)
	if err != nil || started.Status != StatusRunning || started.PID <= 0 {
		t.Fatalf("started=%+v err=%v", started, err)
	}
	waitForLog(t, supervisor, "terminal-stdin", "helper-ready")
	logs := supervisor.CollectLogs("terminal-stdin", 0)
	if !containsLog(logs, "[stdout]") || !containsLog(logs, "[stderr]") {
		t.Fatalf("logs=%v", logs)
	}
	stopped, err := supervisor.StopProcess(context.Background(), "terminal-stdin")
	if err != nil || stopped.Status != StatusStopped || stopped.ExitCode == nil || *stopped.ExitCode != 0 {
		t.Fatalf("stopped=%+v err=%v", stopped, err)
	}
	restarted, err := supervisor.RestartProcess(context.Background(), "terminal-stdin", profile)
	if err != nil || restarted.Status != StatusRunning || restarted.ProcessID == started.ProcessID {
		t.Fatalf("restarted=%+v err=%v", restarted, err)
	}
	_, _ = supervisor.StopProcess(context.Background(), "terminal-stdin")
}

func TestTerminalExecutorStartsProfileLoadedFromTrustedConfig(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	configuration := map[string]any{"runtime_profiles": []map[string]any{{
		"id": "loaded-terminal", "name": "Loaded terminal", "runtime_type": "terminal",
		"command_argv": []string{executable, "-test.run=TestTerminalHelperProcess", "--"}, "working_dir": ".",
		"env":           map[string]string{"STRATUM_TERMINAL_HELPER": "1", "STRATUM_TERMINAL_MODE": "stdin"},
		"stop_strategy": "stdin", "stop_stdin_command": "stop", "graceful_stop_timeout": "1s",
		"force_kill_timeout": "1s", "log_mode": "combined", "enabled": true,
	}}}
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "runtime-profiles.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	profiles, err := runtimeprofile.LoadTrustedFile(path)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	started, err := supervisor.StartProcess(context.Background(), "loaded-terminal", profiles[0])
	if err != nil || started.Status != StatusRunning {
		t.Fatalf("started=%+v err=%v", started, err)
	}
	waitForLog(t, supervisor, "loaded-terminal", "helper-ready")
	if stopped, err := supervisor.StopProcess(context.Background(), "loaded-terminal"); err != nil || stopped.Status != StatusStopped {
		t.Fatalf("stopped=%+v err=%v", stopped, err)
	}
}

func TestTerminalExecutorTerminateAndUnexpectedExit(t *testing.T) {
	supervisor, profile := terminalTestSupervisor(t, "long")
	profile.StopStrategy, profile.GracefulStopTimeout = runtimeprofile.StopTerminate, 20*time.Millisecond
	if _, err := supervisor.StartProcess(context.Background(), "terminal-terminate", profile); err != nil {
		t.Fatal(err)
	}
	stopped, err := supervisor.StopProcess(context.Background(), "terminal-terminate")
	if err != nil || stopped.Status != StatusStopped {
		t.Fatalf("stopped=%+v err=%v", stopped, err)
	}

	exitSupervisor, exitProfile := terminalTestSupervisor(t, "exit")
	if _, err := exitSupervisor.StartProcess(context.Background(), "terminal-exit", exitProfile); err != nil {
		t.Fatal(err)
	}
	status := waitForTerminalStatus(t, exitSupervisor, "terminal-exit")
	if status.Status != StatusCrashed || !status.Crashed || status.ExitCode == nil || *status.ExitCode != 7 {
		t.Fatalf("status=%+v", status)
	}

	normalSupervisor, normalProfile := terminalTestSupervisor(t, "exit-zero")
	if _, err := normalSupervisor.StartProcess(context.Background(), "terminal-exit-zero", normalProfile); err != nil {
		t.Fatal(err)
	}
	normal := waitForTerminalStatus(t, normalSupervisor, "terminal-exit-zero")
	if normal.Status != StatusExited || normal.Crashed || normal.ExitCode == nil || *normal.ExitCode != 0 {
		t.Fatalf("normal=%+v", normal)
	}

	noneSupervisor, noneProfile := terminalTestSupervisor(t, "long")
	noneProfile.StopStrategy, noneProfile.GracefulStopTimeout = runtimeprofile.StopNone, 20*time.Millisecond
	if _, err := noneSupervisor.StartProcess(context.Background(), "terminal-none", noneProfile); err != nil {
		t.Fatal(err)
	}
	if stopped, err := noneSupervisor.StopProcess(context.Background(), "terminal-none"); err != nil || stopped.Status != StatusStopped {
		t.Fatalf("none stopped=%+v err=%v", stopped, err)
	}
}

func TestTerminalWorkingDirectoryAndLogLimit(t *testing.T) {
	supervisor, profile := terminalTestSupervisor(t, "stdin")
	profile.WorkingDir = "../escape"
	if _, err := supervisor.StartProcess(context.Background(), "escape", profile); err == nil {
		t.Fatal("escaping workdir should fail")
	}
	_, profile = terminalTestSupervisor(t, "stdin")
	supervisor, _ = NewSupervisorWithRoot("agent-test", t.TempDir(), 96)
	if _, err := supervisor.StartProcess(context.Background(), "limited", profile); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, supervisor, "limited", "helper-ready")
	logs := supervisor.CollectLogs("limited", 48)
	total := 0
	for _, line := range logs {
		total += len(line) + 1
	}
	if total > 48 {
		t.Fatalf("logs exceed limit: %d %v", total, logs)
	}
	_, _ = supervisor.StopProcess(context.Background(), "limited")
}

func TestTerminalEmptyWorkingDirectoryUsesSessionWorkDir(t *testing.T) {
	supervisor, profile := terminalTestSupervisor(t, "stdin")
	profile.WorkingDir = ""
	started, err := supervisor.StartProcess(context.Background(), "terminal-default-work", profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(started.WorkDir, filepath.Join("sessions", "terminal-default-work", "work")) {
		t.Fatalf("workDir=%q", started.WorkDir)
	}
	waitForLog(t, supervisor, "terminal-default-work", "helper-ready")
	_, _ = supervisor.StopProcess(context.Background(), "terminal-default-work")
}

func TestSupervisorMarkCrashed(t *testing.T) {
	supervisor := NewSupervisor("agent-test")
	_, _ = supervisor.StartProcess(context.Background(), "session-1", runtimeprofile.DummyProcess())
	crashed, err := supervisor.MarkCrashed("session-1", "test exit")
	if err != nil || crashed.Status != StatusCrashed || crashed.ExitCode == nil || crashed.LastError == "" {
		t.Fatalf("crashed=%+v err=%v", crashed, err)
	}
}

func TestMCDRPythonMultiStepGracefulStop(t *testing.T) {
	supervisor, profile := terminalTestSupervisor(t, "stdin")
	profile.RuntimeType = runtimeprofile.TypeMCDRPython
	profile.GracefulStopSteps = []runtimeprofile.GracefulStopStep{
		{Type: runtimeprofile.GracefulStopStdinCommand, Command: "stop", Timeout: time.Second},
		{Type: runtimeprofile.GracefulStopSignal, Signal: "SIGTERM", Timeout: 100 * time.Millisecond},
		{Type: runtimeprofile.GracefulStopSignal, Signal: "SIGKILL", Timeout: time.Second},
	}
	profile.ReadinessCheck = &runtimeprofile.ReadinessCheckConfig{Type: runtimeprofile.ReadinessLogPattern, Pattern: "helper-ready", Timeout: 5 * time.Second}
	profile.HealthCheck = &runtimeprofile.HealthCheckConfig{Type: runtimeprofile.HealthProcessAlive, MaxSilentSeconds: 60}
	started, err := supervisor.StartProcess(context.Background(), "mcdr-multi-stop", profile)
	if err != nil || started.Status != StatusRunning || started.PID <= 0 {
		t.Fatalf("started=%+v err=%v", started, err)
	}
	waitForLog(t, supervisor, "mcdr-multi-stop", "helper-ready")
	stopped, err := supervisor.StopProcess(context.Background(), "mcdr-multi-stop")
	if err != nil || stopped.Status != StatusStopped || stopped.ExitCode == nil || *stopped.ExitCode != 0 {
		t.Fatalf("stopped=%+v err=%v", stopped, err)
	}
}

func TestMCDRPythonShellRejected(t *testing.T) {
	if _, err := runtimeprofile.NewRegistry(runtimeprofile.Profile{
		ID: "mcdr-shell", Name: "Shell", RuntimeType: runtimeprofile.TypeMCDRPython,
		CommandArgv: []string{"bash", "-c", "mcdreforged"}, WorkingDir: ".",
		StopStrategy: runtimeprofile.StopStdin, StopStdinCommand: "stop",
		GracefulStopTimeout: time.Second, ForceKillTimeout: time.Second,
		LogMode: runtimeprofile.LogMemory, Enabled: true,
	}); err == nil {
		t.Fatal("mcdr-python with shell executable should be rejected")
	}
}

func TestSupervisorSendCommand(t *testing.T) {
	supervisor, profile := terminalTestSupervisor(t, "stdin")
	if _, err := supervisor.StartProcess(context.Background(), "send-cmd", profile); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, supervisor, "send-cmd", "helper-ready")
	if err := supervisor.SendCommand("send-cmd", "save-all"); err != nil {
		t.Fatalf("SendCommand failed: %v", err)
	}
	logs := supervisor.CollectLogs("send-cmd", 0)
	if !containsLog(logs, "save-all") {
		t.Fatalf("command not found in logs: %v", logs)
	}
	if err := supervisor.SendCommand("send-cmd", ""); err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("empty command err=%v", err)
	}
	if err := supervisor.SendCommand("send-cmd", "bad\ncommand"); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("control chars err=%v", err)
	}
	if err := supervisor.SendCommand("unknown-session", "save-all"); err == nil || !strings.Contains(err.Error(), "not started") {
		t.Fatalf("unknown session err=%v", err)
	}
	if err := supervisor.SendCommand("send-cmd-dummy", "save-all"); err == nil || !strings.Contains(err.Error(), "not started") {
		t.Fatalf("dummy session err=%v", err)
	}
	_, _ = supervisor.StopProcess(context.Background(), "send-cmd")
	stopped := supervisor.InspectProcess("send-cmd")
	if stopped.Status != StatusStopped {
		t.Fatalf("expected stopped: %+v", stopped)
	}
	if err := supervisor.SendCommand("send-cmd", "save-all"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("stopped session SendCommand should fail: %v", err)
	}
}

func terminalTestSupervisor(t *testing.T, mode string) (*Supervisor, runtimeprofile.Profile) {
	t.Helper()
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	profile := runtimeprofile.Profile{ID: "terminal-test-" + mode, Name: "Terminal test helper", RuntimeType: runtimeprofile.TypeTerminal, CommandArgv: []string{executable, "-test.run=TestTerminalHelperProcess", "--"}, WorkingDir: ".", Env: map[string]string{"STRATUM_TERMINAL_HELPER": "1", "STRATUM_TERMINAL_MODE": mode}, StopStrategy: runtimeprofile.StopStdin, StopStdinCommand: "stop", GracefulStopTimeout: time.Second, ForceKillTimeout: time.Second, LogMode: runtimeprofile.LogMemory, Enabled: true}
	return supervisor, profile
}

func TestTerminalHelperProcess(t *testing.T) {
	if os.Getenv("STRATUM_TERMINAL_HELPER") != "1" {
		return
	}
	fmt.Println("helper-ready stdout")
	fmt.Fprintln(os.Stderr, "helper-ready stderr")
	switch os.Getenv("STRATUM_TERMINAL_MODE") {
	case "stdin":
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if scanner.Text() == "stop" {
				fmt.Println("helper-stopped")
				os.Exit(0)
			}
		}
		os.Exit(2)
	case "exit":
		os.Exit(7)
	case "exit-zero":
		os.Exit(0)
	default:
		time.Sleep(30 * time.Second)
	}
}

func waitForLog(t *testing.T, supervisor *Supervisor, sessionID, text string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if containsLog(supervisor.CollectLogs(sessionID, 0), text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("log %q not found: %v", text, supervisor.CollectLogs(sessionID, 0))
}

func waitForTerminalStatus(t *testing.T, supervisor *Supervisor, sessionID string) RuntimeProcess {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := supervisor.InspectProcess(sessionID)
		if status.Status == StatusExited || status.Status == StatusCrashed {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("terminal did not exit: %+v", supervisor.InspectProcess(sessionID))
	return RuntimeProcess{}
}

func containsLog(lines []string, text string) bool {
	for _, line := range lines {
		if strings.Contains(line, text) {
			return true
		}
	}
	return false
}
