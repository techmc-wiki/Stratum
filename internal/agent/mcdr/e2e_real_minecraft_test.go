//go:build integration

package mcdr

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentprocess "github.com/stratummc/stratum/internal/agent/process"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/agent/serverjar"
)

func TestE2ERealMCDRMinecraftBoot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E Minecraft boot test in short mode")
	}

	// Check Java availability
	javaExec := os.Getenv("E2E_JAVA_EXECUTABLE")
	if javaExec == "" {
		javaExec = "java"
	}
	javaCheck := exec.Command(javaExec, "-version")
	if err := javaCheck.Run(); err != nil {
		t.Skipf("Java not available (tried %q): %v. Set E2E_JAVA_EXECUTABLE or add Java to PATH.", javaExec, err)
	}

	// Check MCDR availability
	mcdrExecutable := os.Getenv("MCDR_EXECUTABLE")
	if mcdrExecutable == "" {
		mcdrExecutable = "mcdreforged"
	}
	mcdrCheck := exec.Command(mcdrExecutable, "--version")
	if err := mcdrCheck.Run(); err != nil {
		t.Skipf("MCDReforged not available (tried %q): %v. Install with: pip install mcdreforged", mcdrExecutable, err)
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
	props := "server-port=25566\nonline-mode=false\nenable-command-block=true\nmax-players=5\ndifficulty=peaceful\n"
	if err := os.WriteFile(propsPath, []byte(props), 0o640); err != nil {
		t.Fatal(err)
	}

	// Download real Fabric server jar
	cacheDir := filepath.Join(root, ".cache")
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		t.Fatal(err)
	}
	deployer := serverjar.NewDeployer(cacheDir)
	t.Logf("Downloading Fabric 1.17.1 server jar (may take 30-60s)...")
	deployResult, err := deployer.Deploy(context.Background(), serverjar.DeployRequest{
		SessionID:        sessionID,
		ServerCore:       "fabric",
		MinecraftVersion: "1.17.1",
		LoaderVersion:    "",
		TargetDir:        mcdrLayout.MCDRServerDir,
	})
	if err != nil {
		t.Fatalf("deploy server jar: %v", err)
	}
	t.Logf("Server jar deployed: %s (%.2f MB)", deployResult.JarName, float64(deployResult.SizeBytes)/1024/1024)

	commandArgv := []string{mcdrExecutable}

	// Write environment materialization manifest for supervisor to read
	manifestPath := filepath.Join(sessionLayout.ConfigDir, "environment-materialization.json")
	manifestData := map[string]string{
		"serverJarName":  deployResult.JarName,
		"javaExecutable": javaExec,
		"mcdrExecutable": mcdrExecutable,
	}
	manifestJSON, err := os.Create(manifestPath)
	if err != nil {
		t.Fatalf("create materialization manifest: %v", err)
	}
	if err := json.NewEncoder(manifestJSON).Encode(manifestData); err != nil {
		manifestJSON.Close()
		t.Fatalf("write materialization manifest: %v", err)
	}
	manifestJSON.Close()

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
			Timeout: 180 * time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	t.Logf("Starting MCDR with Java=%s jar=%s", javaExec, deployResult.JarName)
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
