package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type CommandRunner func(context.Context, string, ...string) (string, error)

type Manager struct {
	Run CommandRunner
}

type VenvRequest struct {
	SessionID     string
	VenvPath      string
	Python        Installation
	ClearExisting bool
}

type VenvResult struct {
	SessionID      string `json:"sessionId"`
	VenvPath       string `json:"venvPath"`
	PythonExec     string `json:"pythonExec"`
	PipExec        string `json:"pipExec"`
	MCDRExecutable string `json:"mcdrExecutable"`
}

type InstallMCDRRequest struct {
	Venv      VenvResult
	Version   string
	IndexURL  string
	ProxyURL  string
	ExtraArgs []string
}

func NewManager() *Manager {
	return &Manager{Run: defaultRun}
}

func (m *Manager) CreateVenv(ctx context.Context, req VenvRequest) (VenvResult, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return VenvResult{}, fmt.Errorf("session id is required")
	}
	venvPath := strings.TrimSpace(req.VenvPath)
	if venvPath == "" {
		return VenvResult{}, fmt.Errorf("venv path is required")
	}
	if req.Python.ExecutablePath == "" {
		return VenvResult{}, fmt.Errorf("python executable path is required")
	}
	if req.ClearExisting {
		if err := os.RemoveAll(venvPath); err != nil {
			return VenvResult{}, fmt.Errorf("remove existing venv: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(venvPath), 0o755); err != nil {
		return VenvResult{}, fmt.Errorf("create venv parent directory: %w", err)
	}
	run := m.runner()
	args := append([]string{}, req.Python.PrefixArgs...)
	args = append(args, "-m", "venv", venvPath)
	if _, err := run(ctx, req.Python.ExecutablePath, args...); err != nil {
		return VenvResult{}, fmt.Errorf("create python venv: %w", err)
	}
	result := BuildVenvResult(req.SessionID, venvPath)
	if _, err := run(ctx, result.PythonExec, "--version"); err != nil {
		return VenvResult{}, fmt.Errorf("verify venv python: %w", err)
	}
	if _, err := run(ctx, result.PipExec, "--version"); err != nil {
		return VenvResult{}, fmt.Errorf("verify venv pip: %w", err)
	}
	return result, nil
}

func (m *Manager) InstallMCDR(ctx context.Context, req InstallMCDRRequest) error {
	if strings.TrimSpace(req.Venv.PipExec) == "" {
		return fmt.Errorf("venv pip executable is required")
	}
	args := []string{"install"}
	if req.IndexURL != "" {
		args = append(args, "-i", req.IndexURL)
	}
	if req.ProxyURL != "" {
		args = append(args, "--proxy", req.ProxyURL)
	}
	args = append(args, req.ExtraArgs...)
	args = append(args, MCDRPackageSpec(req.Version))
	if _, err := m.runner()(ctx, req.Venv.PipExec, args...); err != nil {
		return fmt.Errorf("install MCDR: %w", err)
	}
	return nil
}

func (m *Manager) VerifyMCDR(ctx context.Context, venv VenvResult) (string, error) {
	if strings.TrimSpace(venv.MCDRExecutable) == "" {
		return "", fmt.Errorf("MCDR executable is required")
	}
	output, err := m.runner()(ctx, venv.MCDRExecutable, "--version")
	if err != nil {
		return "", fmt.Errorf("verify MCDR: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func BuildVenvResult(sessionID, venvPath string) VenvResult {
	binDir := "bin"
	pythonName := "python"
	pipName := "pip"
	mcdrName := "mcdreforged"
	if runtime.GOOS == "windows" {
		binDir = "Scripts"
		pythonName = "python.exe"
		pipName = "pip.exe"
		mcdrName = "mcdreforged.exe"
	}
	binPath := filepath.Join(venvPath, binDir)
	return VenvResult{
		SessionID:      sessionID,
		VenvPath:       venvPath,
		PythonExec:     filepath.Join(binPath, pythonName),
		PipExec:        filepath.Join(binPath, pipName),
		MCDRExecutable: filepath.Join(binPath, mcdrName),
	}
}

func MCDRPackageSpec(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || strings.EqualFold(version, "latest") {
		return "mcdreforged"
	}
	return "mcdreforged==" + version
}

func (m *Manager) runner() CommandRunner {
	if m != nil && m.Run != nil {
		return m.Run
	}
	return defaultRun
}
