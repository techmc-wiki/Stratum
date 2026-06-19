package mcdr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentprocess "github.com/stratummc/stratum/internal/agent/process"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

func TestMCDRSupervisorStartStop(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "stdin")

	state, err := ms.Start(context.Background(), "mcdr-start-stop", profile)
	if err != nil || state.Status != StatusRunning || state.PID <= 0 {
		t.Fatalf("started=%+v err=%v", state, err)
	}
	if !ms.IsRunning("mcdr-start-stop") {
		t.Fatal("MCDR should be running after start")
	}

	sessionLayout, err := agentprocess.NewSessionRuntimeLayout(root, "mcdr-start-stop")
	if err != nil {
		t.Fatal(err)
	}
	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{mcdrLayout.MCDRRoot, mcdrLayout.MCDRConfigDir, mcdrLayout.MCDRPluginsDir,
		mcdrLayout.MCDRServerDir, mcdrLayout.MCDRLogsDir, mcdrLayout.MCDRTmpDir} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("MCDR directory %q missing: info=%+v err=%v", path, info, err)
		}
	}
	if _, err := os.Stat(mcdrLayout.MCDRManifestPath); err != nil {
		t.Fatalf("MCDR layout manifest missing: %v", err)
	}

	stopped, err := ms.Stop(context.Background(), "mcdr-start-stop")
	if err != nil || stopped.Status != StatusStopped {
		t.Fatalf("stopped=%+v err=%v", stopped, err)
	}
	if ms.IsRunning("mcdr-start-stop") {
		t.Fatal("MCDR should not be running after stop")
	}
}

func TestMCDRSupervisorStartStopIdempotent(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "stdin")

	_, err = ms.Start(context.Background(), "mcdr-idempotent", profile)
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := ms.Stop(context.Background(), "mcdr-idempotent")
	if err != nil || stopped.Status != StatusStopped {
		t.Fatalf("first stop: %+v err=%v", stopped, err)
	}
	secondStop, err := ms.Stop(context.Background(), "mcdr-idempotent")
	if err != nil {
		t.Fatalf("second stop err=%v", secondStop)
	}
	if secondStop.Status != StatusStopped {
		t.Fatalf("second stop status=%s", secondStop.Status)
	}
}

func TestMCDRSupervisorRestart(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "stdin")

	first, err := ms.Start(context.Background(), "mcdr-restart", profile)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ms.Restart(context.Background(), "mcdr-restart", profile)
	if err != nil || second.Status != StatusRunning || second.PID <= 0 {
		t.Fatalf("restarted=%+v err=%v", second, err)
	}
	if second.PID == first.PID {
		t.Fatal("restart should produce a new PID")
	}
	ms.Stop(context.Background(), "mcdr-restart")
}

func TestMCDRSupervisorSendCommand(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "stdin")

	_, err = ms.Start(context.Background(), "mcdr-send-cmd", profile)
	if err != nil {
		t.Fatal(err)
	}
	waitForLog(t, ps, "mcdr-send-cmd", "helper-ready")

	if err := ms.SendCommand("mcdr-send-cmd", "save-all"); err != nil {
		t.Fatalf("SendCommand: %v", err)
	}
	logs := ms.CollectLogs("mcdr-send-cmd", 0)
	if !containsLog(logs, "save-all") {
		t.Fatalf("command not found in MCDR logs: %v", logs)
	}
	if err := ms.SendCommand("mcdr-send-cmd", ""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty command err=%v", err)
	}

	ms.Stop(context.Background(), "mcdr-send-cmd")
	if err := ms.SendCommand("mcdr-send-cmd", "save-all"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("SendCommand on stopped session should fail: %v", err)
	}
}

func TestMCDRSupervisorRejectsNonMCDRProfile(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	_, err = ms.Start(context.Background(), "bad-profile", runtimeprofile.DummyProcess())
	if err == nil || !strings.Contains(err.Error(), "mcdr-python") {
		t.Fatalf("expected mcdr-python rejection: %v", err)
	}
}

func TestMCDRSupervisorInspectNotStarted(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	state := ms.Inspect("unknown")
	if state.Status != StatusNotStarted {
		t.Fatalf("expected not_started: %+v", state)
	}
}

func mcdrTestProfile(t *testing.T, mode string) runtimeprofile.Profile {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return runtimeprofile.Profile{
		ID: "mcdr-test-" + mode, Name: "MCDR test", RuntimeType: runtimeprofile.TypeMCDRPython,
		CommandArgv: []string{executable, "-test.run=TestMCDRHelperProcess", "--"},
		WorkingDir:  ".", Env: map[string]string{"STRATUM_MCDR_HELPER": "1", "STRATUM_MCDR_MODE": mode},
		StopStrategy: runtimeprofile.StopStdin, StopStdinCommand: "stop",
		GracefulStopTimeout: time.Second, ForceKillTimeout: time.Second,
		LogMode: runtimeprofile.LogMemory, Enabled: true,
	}
}

func TestMCDRHelperProcess(t *testing.T) {
	if os.Getenv("STRATUM_MCDR_HELPER") != "1" {
		return
	}
	switch os.Getenv("STRATUM_MCDR_MODE") {
	case "stdin":
		os.Stdout.WriteString("helper-ready\n")
		os.Stderr.WriteString("helper-stderr\n")
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				os.Exit(0)
			}
			if strings.TrimSpace(string(buf[:n])) == "stop" {
				os.Stdout.WriteString("helper-stopped\n")
				os.Exit(0)
			}
		}
	case "long":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "exit":
		os.Exit(7)
	case "exit-zero":
		os.Exit(0)
	default:
		os.Exit(0)
	}
}

func waitForLog(t *testing.T, supervisor *agentprocess.Supervisor, sessionID, text string) {
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

func containsLog(lines []string, text string) bool {
	for _, line := range lines {
		if strings.Contains(line, text) {
			return true
		}
	}
	return false
}

func TestMCDRSupervisorStartWritesConfigYML(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "stdin")
	state, err := ms.Start(context.Background(), "mcdr-config-test", profile)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusRunning {
		t.Fatalf("expected running, got %s", state.Status)
	}
	sessionLayout, err := agentprocess.NewSessionRuntimeLayout(root, "mcdr-config-test")
	if err != nil {
		t.Fatal(err)
	}
	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(mcdrLayout.MCDRConfigDir, configYMLName)
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config.yml not written: %v", err)
	}
	content := string(payload)
	if !strings.Contains(content, "working_directory") || !strings.Contains(content, "plugin_directories") {
		t.Fatalf("config.yml missing expected keys: %s", content)
	}
	ms.Stop(context.Background(), "mcdr-config-test")
}

func TestMCDRSupervisorStartConfigWriteFailsDoesNotStartProcess(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "stdin")
	sessionLayout, err := agentprocess.NewSessionRuntimeLayout(root, "mcdr-fail-config")
	if err != nil {
		t.Fatal(err)
	}
	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	if err := mcdrLayout.Create(); err != nil {
		t.Fatal(err)
	}
	if err := mcdrLayout.WriteManifest(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mcdrLayout.MCDRConfigDir, 0o750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(mcdrLayout.MCDRConfigDir, configYMLName)
	if err := os.WriteFile(configPath, []byte{}, 0o400); err != nil {
		t.Fatal(err)
	}
	state, err := ms.Start(context.Background(), "mcdr-fail-config", profile)
	if err == nil || !strings.Contains(err.Error(), "config.yml") {
		t.Fatalf("expected config write failure, got state=%+v err=%v", state, err)
	}
	if ms.IsRunning("mcdr-fail-config") {
		t.Fatal("process should not start when config write fails")
	}
}

func TestMCDRSupervisorStartWithReadinessCheckSuccess(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "stdin")
	profile.ReadinessCheck = &runtimeprofile.ReadinessCheckConfig{
		Type:    runtimeprofile.ReadinessLogPattern,
		Pattern: "helper-ready",
		Timeout: 3 * time.Second,
	}
	state, err := ms.Start(context.Background(), "mcdr-ready-ok", profile)
	if err != nil {
		t.Fatalf("Start with readiness: %v", err)
	}
	if state.Status != StatusRunning {
		t.Fatalf("expected running, got %s", state.Status)
	}
	ms.Stop(context.Background(), "mcdr-ready-ok")
}

func TestMCDRSupervisorStartWithReadinessCheckTimeout(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "long")
	profile.ReadinessCheck = &runtimeprofile.ReadinessCheckConfig{
		Type:    runtimeprofile.ReadinessLogPattern,
		Pattern: "helper-ready",
		Timeout: 200 * time.Millisecond,
	}
	_, err = ms.Start(context.Background(), "mcdr-ready-timeout", profile)
	if err == nil || !strings.Contains(err.Error(), "readiness") {
		t.Fatalf("expected readiness timeout, got %v", err)
	}
	if ms.IsRunning("mcdr-ready-timeout") {
		t.Fatal("process should be stopped after readiness timeout")
	}
}

func TestMCDRSupervisorStartWithReadinessCheckExit(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "exit")
	profile.ReadinessCheck = &runtimeprofile.ReadinessCheckConfig{
		Type:    runtimeprofile.ReadinessLogPattern,
		Pattern: "helper-ready",
		Timeout: 3 * time.Second,
	}
	_, err = ms.Start(context.Background(), "mcdr-ready-exit", profile)
	if err == nil || !strings.Contains(err.Error(), "readiness") {
		t.Fatalf("expected readiness exit detection, got %v", err)
	}
	if ms.IsRunning("mcdr-ready-exit") {
		t.Fatal("process should not be running after crash during readiness")
	}
}
