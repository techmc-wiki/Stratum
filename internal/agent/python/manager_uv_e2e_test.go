package python

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2E_UVManager_CreateVenvAndInstallMCDR(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv not installed, skipping e2e test")
	}

	detector := NewDetector()
	python, err := detector.SelectForMCDR(ctx)
	if err != nil {
		t.Fatalf("detect python: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "stratum-e2e-uv-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	venvPath := filepath.Join(tmpDir, "venv")

	manager := &Manager{
		Run:         defaultRun,
		ManagerType: ManagerUV,
	}

	t.Logf("Creating venv with uv at %s using Python %s", venvPath, python.Version)
	venv, err := manager.CreateVenv(ctx, VenvRequest{
		SessionID:     "e2e-uv-test",
		VenvPath:      venvPath,
		Python:        python,
		ClearExisting: false,
		ManagerType:   ManagerUV,
	})
	if err != nil {
		t.Fatalf("create venv with uv: %v", err)
	}

	if venv.VenvPath != venvPath {
		t.Errorf("venv path: got %q, want %q", venv.VenvPath, venvPath)
	}

	if _, err := os.Stat(venv.PythonExec); err != nil {
		t.Errorf("python executable not found at %s: %v", venv.PythonExec, err)
	}

	t.Logf("Installing MCDR with uv")
	installReq := InstallMCDRRequest{
		Venv:        venv,
		Version:     "",
		ManagerType: ManagerUV,
	}

	if err := manager.InstallMCDR(ctx, installReq); err != nil {
		t.Fatalf("install MCDR with uv: %v", err)
	}

	t.Logf("Verifying MCDR installation")
	version, err := manager.VerifyMCDR(ctx, venv)
	if err != nil {
		t.Fatalf("verify MCDR: %v", err)
	}

	if !strings.Contains(strings.ToLower(version), "mcdr") {
		t.Errorf("unexpected MCDR version output: %q", version)
	}

	t.Logf("MCDR installed successfully: %s", version)

	if _, err := os.Stat(venv.MCDRExecutable); err != nil {
		t.Errorf("MCDR executable not found at %s: %v", venv.MCDRExecutable, err)
	}
}
