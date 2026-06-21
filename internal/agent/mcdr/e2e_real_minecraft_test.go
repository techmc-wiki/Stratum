//go:build integration

package mcdr

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentprocess "github.com/stratummc/stratum/internal/agent/process"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

func TestE2ERealMCDRMinecraftBoot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E Minecraft boot test in short mode")
	}

	javaHome := os.Getenv("JAVA_HOME")
	path := os.Getenv("PATH")
	if javaHome == "" && !strings.Contains(strings.ToLower(path), "java") {
		t.Skip("Java not detected (set JAVA_HOME or add java to PATH)")
	}

	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-e2e", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)

	sessionID := "e2e-minecraft"
	sessionLayout, err := agentprocess.NewSessionRuntimeLayout(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := sessionLayout.Create(); err != nil {
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
	if err := os.MkdirAll(mcdrLayout.MCDRServerDir, 0o750); err != nil {
		t.Fatal(err)
	}

	eulaPath := filepath.Join(mcdrLayout.MCDRServerDir, "eula.txt")
	if err := os.WriteFile(eulaPath, []byte("eula=true\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	propsPath := filepath.Join(mcdrLayout.MCDRServerDir, "server.properties")
	props := "server-port=25566\nonline-mode=false\nenable-command-block=true\nmax-players=5\n"
	if err := os.WriteFile(propsPath, []byte(props), 0o640); err != nil {
		t.Fatal(err)
	}

	mcdrExecutable := os.Getenv("MCDR_EXECUTABLE")
	if mcdrExecutable == "" {
		mcdrExecutable = "mcdreforged"
	}

	commandArgv := []string{mcdrExecutable}
	serverJarName := os.Getenv("E2E_SERVER_JAR")
	if serverJarName == "" {
		serverJarName = "fabric-server-launch.jar"
	}
	javaExec := os.Getenv("E2E_JAVA_EXECUTABLE")
	if javaExec == "" {
		javaExec = "java"
	}

	mcdrConfig := NewRuntimeConfig(mcdrLayout)
	mcdrConfig.ServerJarName = serverJarName
	mcdrConfig.JavaExecutable = javaExec

	if _, err := WriteRuntimeConfig(mcdrLayout, mcdrConfig); err != nil {
		t.Fatalf("write MCDR config.yml: %v", err)
	}

	profile := runtimeprofile.Profile{
		ID:                  "mcdr-e2e",
		Name:                "MCDR E2E",
		RuntimeType:         runtimeprofile.TypeMCDRPython,
		CommandArgv:         commandArgv,
		WorkingDir:          "",
		StopStrategy:        runtimeprofile.StopStdin,
		StopStdinCommand:    "stop",
		GracefulStopTimeout: 30 * time.Second,
		ForceKillTimeout:    15 * time.Second,
		LogMode:             runtimeprofile.LogMemory,
		Enabled:             true,
		ReadinessCheck: &runtimeprofile.ReadinessCheckConfig{
			Type:    runtimeprofile.ReadinessLogPattern,
			Pattern: "Done (",
			Timeout: 120 * time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Logf("Starting MCDR with Java=%s jar=%s", javaExec, serverJarName)
	state, err := ms.Start(ctx, sessionID, profile)
	if err != nil {
		if strings.Contains(err.Error(), "readiness") {
			logs := ms.CollectLogs(sessionID, 0)
			for _, line := range logs {
				t.Logf("MCDR log: %s", line)
			}
		}
		t.Fatalf("MCDR start failed: %v", err)
	}
	t.Logf("MCDR running: PID=%d status=%s", state.PID, state.Status)

	logs := ms.CollectLogs(sessionID, 0)
	foundDone := false
	for _, line := range logs {
		if strings.Contains(line, "Done (") {
			foundDone = true
			break
		}
	}
	if !foundDone {
		t.Error("Minecraft 'Done (' readiness pattern not found in logs")
	}

	t.Logf("Stopping MCDR gracefully...")
	stopped, err := ms.Stop(ctx, sessionID)
	if err != nil {
		t.Fatalf("MCDR stop failed: %v", err)
	}
	t.Logf("MCDR stopped: status=%s exitCode=%v", stopped.Status, stopped.ExitCode)

	if ms.IsRunning(sessionID) {
		t.Fatal("MCDR process should not be running after stop")
	}
}

func TestE2ERealMCDRMinecraftBootWithProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E Minecraft boot test in short mode")
	}

	proxyURL := os.Getenv("STRATUM_HTTP_PROXY")
	if proxyURL == "" {
		t.Skip("STRATUM_HTTP_PROXY not set")
	}

	resp, err := http.Get(proxyURL)
	if err != nil || resp == nil {
		t.Skipf("proxy %q unreachable: %v", proxyURL, err)
	}
	if resp.Body != nil {
		resp.Body.Close()
	}

	TestE2ERealMCDRMinecraftBoot(t)
}
