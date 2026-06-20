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
	Run         CommandRunner
	ManagerType ManagerType
}

type VenvRequest struct {
	SessionID     string
	VenvPath      string
	Python        Installation
	ClearExisting bool
	ManagerType   ManagerType
}

type VenvResult struct {
	SessionID      string `json:"sessionId"`
	VenvPath       string `json:"venvPath"`
	PythonExec     string `json:"pythonExec"`
	PipExec        string `json:"pipExec"`
	MCDRExecutable string `json:"mcdrExecutable"`
}

type MCDRExecutable struct {
	Executable string `json:"executable"`
	Version    string `json:"version"`
	Source     string `json:"source"`
}

type InstallMCDRRequest struct {
	Venv        VenvResult
	Version     string
	IndexURL    string
	ProxyURL    string
	ExtraArgs   []string
	ManagerType ManagerType
}

func NewManager() *Manager {
	return &Manager{Run: defaultRun, ManagerType: ManagerUV}
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

	managerType := req.ManagerType
	if managerType == "" {
		managerType = m.ManagerType
	}
	if managerType == "" {
		managerType = ManagerVenv
	}

	var err error
	switch managerType {
	case ManagerUV:
		err = m.createVenvWithUV(ctx, req)
	default:
		err = m.createVenvWithStandard(ctx, req)
	}
	if err != nil {
		return VenvResult{}, err
	}

	result := BuildVenvResult(req.SessionID, venvPath)
	run := m.runner()
	if _, err := run(ctx, result.PythonExec, "--version"); err != nil {
		return VenvResult{}, fmt.Errorf("verify venv python: %w", err)
	}
	if managerType != ManagerUV {
		if _, err := run(ctx, result.PipExec, "--version"); err != nil {
			return VenvResult{}, fmt.Errorf("verify venv pip: %w", err)
		}
	}
	return result, nil
}

func (m *Manager) createVenvWithStandard(ctx context.Context, req VenvRequest) error {
	run := m.runner()
	args := append([]string{}, req.Python.PrefixArgs...)
	args = append(args, "-m", "venv", req.VenvPath)
	if _, err := run(ctx, req.Python.ExecutablePath, args...); err != nil {
		return fmt.Errorf("create python venv: %w", err)
	}
	return nil
}

func (m *Manager) createVenvWithUV(ctx context.Context, req VenvRequest) error {
	detector := NewUVDetector()
	if _, err := detector.Detect(ctx); err != nil {
		return err
	}
	run := m.runner()
	args := []string{"venv", req.VenvPath, "--python", req.Python.ExecutablePath}
	if _, err := run(ctx, "uv", args...); err != nil {
		return fmt.Errorf("create uv venv: %w", err)
	}
	return nil
}

func (m *Manager) InstallMCDR(ctx context.Context, req InstallMCDRRequest) error {
	managerType := req.ManagerType
	if managerType == "" {
		managerType = m.ManagerType
	}
	if managerType == "" {
		managerType = ManagerVenv
	}

	switch managerType {
	case ManagerUV:
		return m.installMCDRWithUV(ctx, req)
	default:
		return m.installMCDRWithPip(ctx, req)
	}
}

func (m *Manager) installMCDRWithPip(ctx context.Context, req InstallMCDRRequest) error {
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

func (m *Manager) installMCDRWithUV(ctx context.Context, req InstallMCDRRequest) error {
	detector := NewUVDetector()
	if _, err := detector.Detect(ctx); err != nil {
		return err
	}
	args := []string{"pip", "install", "--python", req.Venv.PythonExec}
	if req.IndexURL != "" {
		args = append(args, "--index-url", req.IndexURL)
	}
	args = append(args, req.ExtraArgs...)
	args = append(args, MCDRPackageSpec(req.Version))
	if _, err := m.runner()(ctx, "uv", args...); err != nil {
		return fmt.Errorf("install MCDR with uv: %w", err)
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

func (m *Manager) VerifyMCDRExecutable(ctx context.Context, executable string) (string, error) {
	if strings.TrimSpace(executable) == "" {
		return "", fmt.Errorf("MCDR executable is required")
	}
	output, err := m.runner()(ctx, executable, "--version")
	if err != nil {
		return "", fmt.Errorf("verify MCDR executable: %w", err)
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
