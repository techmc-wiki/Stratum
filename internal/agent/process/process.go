package process

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	agentjava "github.com/stratummc/stratum/internal/agent/java"
	agentpython "github.com/stratummc/stratum/internal/agent/python"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/agent/serverjar"
	"github.com/stratummc/stratum/internal/agent/serverproperties"
	"github.com/stratummc/stratum/internal/integration/lucy"
	"github.com/stratummc/stratum/internal/safepath"
)

type Status string

const (
	StatusNotStarted Status = "not_started"
	StatusStarting   Status = "starting"
	StatusRunning    Status = "running"
	StatusStopping   Status = "stopping"
	StatusStopped    Status = "stopped"
	StatusExited     Status = "exited"
	StatusCrashed    Status = "crashed"
	StatusUnknown    Status = "unknown"
)

const (
	RuntimeModeDummy    = "dummy-process"
	RuntimeModeTerminal = "managed-terminal"
	defaultLogBytes     = 256 * 1024
)

type RuntimeProcess struct {
	ProcessID        string     `json:"processId"`
	SessionID        string     `json:"sessionId"`
	AgentID          string     `json:"agentId"`
	Status           Status     `json:"status"`
	PID              int        `json:"pid,omitempty"`
	Command          string     `json:"command"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	StoppedAt        *time.Time `json:"stoppedAt,omitempty"`
	ExitCode         *int       `json:"exitCode,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
	LogRef           string     `json:"logRef"`
	RuntimeMode      string     `json:"runtimeMode"`
	RuntimeProfileID string     `json:"runtimeProfileId"`
	RuntimeType      string     `json:"runtimeType"`
	Crashed          bool       `json:"crashed"`
	SessionRoot      string     `json:"sessionRoot,omitempty"`
	WorkDir          string     `json:"workDir,omitempty"`
	LogsDir          string     `json:"logsDir,omitempty"`
}

type logBuffer struct {
	lines           []string
	bytes, maxBytes int
}

func newLogBuffer(maxBytes int) *logBuffer {
	if maxBytes <= 0 {
		maxBytes = defaultLogBytes
	}
	return &logBuffer{maxBytes: maxBytes}
}

func (b *logBuffer) append(line string) {
	lineBytes := len(line) + 1
	if lineBytes > b.maxBytes {
		line = line[len(line)-b.maxBytes+1:]
		lineBytes = len(line) + 1
	}
	b.lines = append(b.lines, line)
	b.bytes += lineBytes
	for b.bytes > b.maxBytes && len(b.lines) > 1 {
		b.bytes -= len(b.lines[0]) + 1
		b.lines = b.lines[1:]
	}
}

func (b *logBuffer) tail(maxBytes int) []string {
	if maxBytes <= 0 || maxBytes >= b.bytes {
		return append([]string(nil), b.lines...)
	}
	used, start := 0, len(b.lines)
	for start > 0 {
		size := len(b.lines[start-1]) + 1
		if used+size > maxBytes {
			break
		}
		used += size
		start--
	}
	if start == len(b.lines) && len(b.lines) > 0 {
		line := b.lines[len(b.lines)-1]
		limit := maxBytes - 1
		if limit <= 0 {
			return nil
		}
		if len(line) > limit {
			line = line[len(line)-limit:]
		}
		return []string{line}
	}
	return append([]string(nil), b.lines[start:]...)
}

type streamWriter struct {
	supervisor *Supervisor
	sessionID  string
	item       *managedProcess
	source     string
}

func (w streamWriter) Write(data []byte) (int, error) {
	text := strings.TrimRight(string(data), "\r\n")
	if text != "" {
		w.supervisor.appendLog(w.sessionID, w.item, "["+w.source+"] "+text)
	}
	return len(data), nil
}

type managedProcess struct {
	model         RuntimeProcess
	profile       runtimeprofile.Profile
	logs          *logBuffer
	cancel        context.CancelFunc
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	done          chan struct{}
	stopRequested bool
}

type Supervisor struct {
	mu                sync.RWMutex
	agentID           string
	runtimeRoot       string
	now               func() time.Time
	processes         map[string]*managedProcess
	sequence          uint64
	maxLogBytes       int
	lucyAdapter       lucy.Adapter
	pythonDetector    pythonRuntimeDetector
	pythonManager     pythonEnvironmentManager
	javaDetector      javaRuntimeDetector
	serverJarDeployer serverJarMaterializer
}

type pythonRuntimeDetector interface {
	SelectForMCDR(context.Context) (agentpython.Installation, error)
}

type pythonEnvironmentManager interface {
	CreateVenv(context.Context, agentpython.VenvRequest) (agentpython.VenvResult, error)
	InstallMCDR(context.Context, agentpython.InstallMCDRRequest) error
	VerifyMCDR(context.Context, agentpython.VenvResult) (string, error)
	VerifyMCDRExecutable(context.Context, string) (string, error)
}

type javaRuntimeDetector interface {
	SelectForMinecraftVersion(context.Context, string) (agentjava.Installation, error)
}

type serverJarMaterializer interface {
	Deploy(context.Context, serverjar.DeployRequest) (serverjar.DeployResult, error)
}

func NewSupervisor(agentID string) *Supervisor {
	root := filepath.Join(os.TempDir(), "stratum-runtime-"+safeName(agentID))
	supervisor, err := NewSupervisorWithRoot(agentID, root, defaultLogBytes)
	if err != nil {
		panic(err)
	}
	return supervisor
}

func NewSupervisorWithRoot(agentID, root string, maxLogBytes int) (*Supervisor, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("runtime root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime root: %w", err)
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create runtime root: %w", err)
	}
	if maxLogBytes <= 0 {
		maxLogBytes = defaultLogBytes
	}
	_ = serverjar.SetProxy(os.Getenv("STRATUM_PROXY"))
	cacheDir := filepath.Join(root, "cache", "serverjars")
	adapter := createDefaultLucyAdapter(root)
	return &Supervisor{agentID: agentID, runtimeRoot: filepath.Clean(root), now: func() time.Time { return time.Now().UTC() }, processes: map[string]*managedProcess{}, maxLogBytes: maxLogBytes, lucyAdapter: adapter, pythonDetector: agentpython.NewDetector(), pythonManager: agentpython.NewManager(), javaDetector: agentjava.NewDetector(), serverJarDeployer: serverjar.NewDeployer(cacheDir)}, nil
}

func createDefaultLucyAdapter(workDir string) lucy.Adapter {
	configured := strings.TrimSpace(os.Getenv("STRATUM_LUCY_WORKSPACE"))
	if strings.EqualFold(configured, "none") {
		return lucy.NoopAdapter{}
	}
	if configured != "" {
		workDir = configured
	}
	adapter, err := lucy.NewEmbeddedAdapter(lucy.NewLucyProjectBackend(workDir, ""))
	if err != nil {
		return lucy.NoopAdapter{}
	}
	return adapter
}

func (s *Supervisor) SetLucyAdapter(adapter lucy.Adapter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if adapter == nil {
		s.lucyAdapter = lucy.NoopAdapter{}
	} else {
		s.lucyAdapter = adapter
	}
}

func (s *Supervisor) SetPythonRuntime(detector pythonRuntimeDetector, manager pythonEnvironmentManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if detector == nil {
		s.pythonDetector = agentpython.NewDetector()
	} else {
		s.pythonDetector = detector
	}
	if manager == nil {
		s.pythonManager = agentpython.NewManager()
	} else {
		s.pythonManager = manager
	}
}

func (s *Supervisor) SetJavaAndServerJarRuntime(detector javaRuntimeDetector, deployer serverJarMaterializer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if detector == nil {
		s.javaDetector = agentjava.NewDetector()
	} else {
		s.javaDetector = detector
	}
	if deployer == nil {
		cacheDir := filepath.Join(s.runtimeRoot, "cache", "serverjars")
		s.serverJarDeployer = serverjar.NewDeployer(cacheDir)
	} else {
		s.serverJarDeployer = deployer
	}
}

func (s *Supervisor) StartProcess(ctx context.Context, sessionID string, profile runtimeprofile.Profile) (RuntimeProcess, error) {
	if sessionID == "" {
		return RuntimeProcess{}, errors.New("session id is required")
	}
	if err := ctx.Err(); err != nil {
		return RuntimeProcess{}, err
	}
	if err := runtimeprofile.Validate(profile); err != nil {
		return RuntimeProcess{}, err
	}
	layout, err := NewSessionRuntimeLayout(s.runtimeRoot, sessionID)
	if err != nil {
		return RuntimeProcess{}, err
	}
	if err := layout.Create(); err != nil {
		return RuntimeProcess{}, err
	}
	workDir := layout.WorkDir
	if profile.RuntimeType == runtimeprofile.TypeTerminal || profile.RuntimeType == runtimeprofile.TypeMCDRPython {
		workDir, err = s.resolveWorkingDir(layout, profile.WorkingDir)
		if err != nil {
			return RuntimeProcess{}, err
		}
	}

	s.mu.Lock()
	if current := s.processes[sessionID]; current != nil && (current.model.Status == StatusStarting || current.model.Status == StatusRunning || current.model.Status == StatusStopping) {
		s.mu.Unlock()
		return RuntimeProcess{}, fmt.Errorf("session %q process is already %s", sessionID, current.model.Status)
	}
	s.sequence++
	now := s.now()
	logs := newLogBuffer(s.maxLogBytes)
	if previous := s.processes[sessionID]; previous != nil {
		logs = previous.logs
		logs.append(formatLog(now, "supervisor", "restart boundary"))
	}
	mode := RuntimeModeDummy
	command := "stratum-dummy-runtime"
	if profile.RuntimeType == runtimeprofile.TypeTerminal || profile.RuntimeType == runtimeprofile.TypeMCDRPython {
		mode, command = RuntimeModeTerminal, profile.ID
	}
	item := &managedProcess{model: RuntimeProcess{ProcessID: fmt.Sprintf("process-%d", s.sequence), SessionID: sessionID, AgentID: s.agentID, Status: StatusStarting, Command: command, StartedAt: &now, LogRef: "memory://runtime/" + sessionID, RuntimeMode: mode, RuntimeProfileID: profile.ID, RuntimeType: string(profile.RuntimeType), SessionRoot: layout.SessionRoot, WorkDir: workDir, LogsDir: layout.LogsDir}, profile: profile, logs: logs, done: make(chan struct{})}
	item.logs.append(formatLog(now, "supervisor", "starting "+profile.ID))
	s.processes[sessionID] = item
	s.mu.Unlock()

	if profile.RuntimeType == runtimeprofile.TypeDummy {
		return s.startDummy(sessionID, item)
	}
	return s.startTerminal(sessionID, item, workDir)
}

func (s *Supervisor) startDummy(sessionID string, item *managedProcess) (RuntimeProcess, error) {
	runCtx, cancel := context.WithCancel(context.Background())
	item.cancel = cancel
	s.mu.Lock()
	item.model.Status = StatusRunning
	item.logs.append(formatLog(s.now(), "dummy-runtime", "runtime running"))
	model := item.model
	s.mu.Unlock()
	go func() { <-runCtx.Done(); s.finishDummy(sessionID, item) }()
	return model, nil
}

func (s *Supervisor) startTerminal(sessionID string, item *managedProcess, workDir string) (RuntimeProcess, error) {
	cmd := exec.Command(item.profile.CommandArgv[0], item.profile.CommandArgv[1:]...)
	cmd.Dir = workDir
	cmd.Env = trustedEnvironment(item.profile.Env)
	cmd.Stdout = streamWriter{s, sessionID, item, "stdout"}
	cmd.Stderr = streamWriter{s, sessionID, item, "stderr"}
	if item.profile.StopStrategy == runtimeprofile.StopStdin || len(item.profile.GracefulStopSteps) > 0 {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return s.failStart(sessionID, item, err)
		}
		item.stdin = stdin
	}
	item.cmd = cmd
	if err := cmd.Start(); err != nil {
		return s.failStart(sessionID, item, err)
	}
	s.mu.Lock()
	item.model.Status, item.model.PID = StatusRunning, cmd.Process.Pid
	item.logs.append(formatLog(s.now(), "supervisor", fmt.Sprintf("terminal running pid=%d", cmd.Process.Pid)))
	model := item.model
	s.mu.Unlock()
	go s.waitTerminal(sessionID, item)
	return model, nil
}

func (s *Supervisor) failStart(sessionID string, item *managedProcess, startErr error) (RuntimeProcess, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now, code := s.now(), -1
	item.model.Status, item.model.StoppedAt, item.model.ExitCode, item.model.LastError = StatusCrashed, &now, &code, startErr.Error()
	item.model.Crashed = true
	item.logs.append(formatLog(now, "supervisor", "start failed: "+startErr.Error()))
	close(item.done)
	return item.model, startErr
}

func (s *Supervisor) StopProcess(ctx context.Context, sessionID string) (RuntimeProcess, error) {
	s.mu.Lock()
	item := s.processes[sessionID]
	if item == nil {
		s.mu.Unlock()
		return RuntimeProcess{}, fmt.Errorf("session %q process is not started", sessionID)
	}
	if item.model.Status != StatusRunning {
		model := item.model
		s.mu.Unlock()
		if model.Status == StatusStopped || model.Status == StatusExited || model.Status == StatusCrashed {
			return model, nil
		}
		return RuntimeProcess{}, fmt.Errorf("session %q process cannot stop from %s", sessionID, model.Status)
	}
	item.model.Status, item.stopRequested = StatusStopping, true
	item.logs.append(formatLog(s.now(), "supervisor", "stop requested"))
	done, profile, cmd, stdin, cancel := item.done, item.profile, item.cmd, item.stdin, item.cancel
	s.mu.Unlock()

	if profile.RuntimeType == runtimeprofile.TypeDummy {
		cancel()
	} else if len(profile.GracefulStopSteps) > 0 {
		for i, step := range profile.GracefulStopSteps {
			switch step.Type {
			case runtimeprofile.GracefulStopStdinCommand:
				if stdin != nil {
					_, _ = io.WriteString(stdin, step.Command+"\n")
				}
			case runtimeprofile.GracefulStopSignal:
				sig := signalByName(step.Signal)
				if cmd != nil && cmd.Process != nil && sig != nil {
					_ = cmd.Process.Signal(sig)
				}
			}
			timeout := step.Timeout
			if timeout <= 0 {
				timeout = 100 * time.Millisecond
			}
			select {
			case <-done:
				return s.InspectProcess(sessionID), nil
			case <-time.After(timeout):
			case <-ctx.Done():
				return RuntimeProcess{}, fmt.Errorf("stop session %q process step %d (%s): %w", sessionID, i, step.Type, ctx.Err())
			}
		}
		return RuntimeProcess{}, fmt.Errorf("session %q process did not exit after multi-step graceful stop", sessionID)
	} else {
		switch profile.StopStrategy {
		case runtimeprofile.StopStdin:
			if stdin != nil {
				_, _ = io.WriteString(stdin, profile.StopStdinCommand+"\n")
			}
		case runtimeprofile.StopTerminate:
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Signal(os.Interrupt)
			}
		}
	}
	grace := profile.GracefulStopTimeout
	if grace <= 0 {
		grace = 100 * time.Millisecond
	}
	select {
	case <-done:
		return s.InspectProcess(sessionID), nil
	case <-time.After(grace):
	case <-ctx.Done():
		return RuntimeProcess{}, fmt.Errorf("stop session %q process: %w", sessionID, ctx.Err())
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	force := profile.ForceKillTimeout
	if force <= 0 {
		force = time.Second
	}
	select {
	case <-done:
		return s.InspectProcess(sessionID), nil
	case <-time.After(force):
		return RuntimeProcess{}, fmt.Errorf("session %q process did not exit after force kill", sessionID)
	case <-ctx.Done():
		return RuntimeProcess{}, fmt.Errorf("stop session %q process: %w", sessionID, ctx.Err())
	}
}

func (s *Supervisor) RestartProcess(ctx context.Context, sessionID string, profile runtimeprofile.Profile) (RuntimeProcess, error) {
	if s.IsRunning(sessionID) {
		if _, err := s.StopProcess(ctx, sessionID); err != nil {
			return RuntimeProcess{}, err
		}
	}
	return s.StartProcess(ctx, sessionID, profile)
}

func (s *Supervisor) InspectProcess(sessionID string) RuntimeProcess {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if item := s.processes[sessionID]; item != nil {
		return item.model
	}
	return RuntimeProcess{SessionID: sessionID, AgentID: s.agentID, Status: StatusNotStarted, Command: "stratum-dummy-runtime", LogRef: "memory://runtime/" + sessionID, RuntimeMode: RuntimeModeDummy, RuntimeProfileID: runtimeprofile.DefaultProfileID, RuntimeType: string(runtimeprofile.TypeDummy)}
}

func (s *Supervisor) CollectLogs(sessionID string, maxBytes int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if item := s.processes[sessionID]; item != nil {
		return item.logs.tail(maxBytes)
	}
	return nil
}

func (s *Supervisor) SendCommand(sessionID, command string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("command is required")
	}
	if strings.ContainsAny(command, "\r\n\x00") {
		return errors.New("command contains control characters")
	}
	s.mu.RLock()
	item := s.processes[sessionID]
	s.mu.RUnlock()
	if item == nil {
		return fmt.Errorf("session %q process is not started", sessionID)
	}
	if item.model.Status != StatusRunning {
		return fmt.Errorf("session %q process is not running (status=%s)", sessionID, item.model.Status)
	}
	if item.stdin == nil {
		return fmt.Errorf("session %q process has no stdin pipe", sessionID)
	}
	_, err := io.WriteString(item.stdin, command+"\n")
	if err != nil {
		return fmt.Errorf("write command to session %q stdin: %w", sessionID, err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if current := s.processes[sessionID]; current == item {
		current.logs.append(formatLog(time.Now().UTC(), "send-command", command))
	}
	return nil
}

func (s *Supervisor) WaitForLog(sessionID, pattern string, timeout time.Duration) error {
	if strings.TrimSpace(pattern) == "" {
		return errors.New("pattern is required")
	}
	if timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	deadline := time.Now().Add(timeout)
	tickInterval := 50 * time.Millisecond
	if timeout < tickInterval {
		tickInterval = timeout / 4
		if tickInterval < time.Millisecond {
			tickInterval = time.Millisecond
		}
	}
	for {
		model := s.InspectProcess(sessionID)
		if model.Status == StatusCrashed || model.Status == StatusExited {
			return fmt.Errorf("process exited before readiness pattern: status=%s exitCode=%v", model.Status, model.ExitCode)
		}
		if model.Status == StatusStopped {
			return fmt.Errorf("process stopped before readiness pattern")
		}
		logs := s.CollectLogs(sessionID, 0)
		for _, line := range logs {
			if strings.Contains(line, pattern) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("readiness pattern %q not found within %v", pattern, timeout)
		}
		time.Sleep(tickInterval)
	}
}

func (s *Supervisor) IsRunning(sessionID string) bool {
	return s.InspectProcess(sessionID).Status == StatusRunning
}

func (s *Supervisor) MarkCrashed(sessionID, message string) (RuntimeProcess, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.processes[sessionID]
	if item == nil {
		return RuntimeProcess{}, fmt.Errorf("session %q process is not started", sessionID)
	}
	if item.cancel != nil {
		item.cancel()
	}
	if item.cmd != nil && item.cmd.Process != nil {
		_ = item.cmd.Process.Kill()
	}
	now, code := s.now(), 1
	item.model.Status, item.model.StoppedAt, item.model.ExitCode, item.model.LastError, item.model.Crashed = StatusCrashed, &now, &code, message, true
	item.logs.append(formatLog(now, "supervisor", "runtime crashed: "+message))
	return item.model, nil
}

func (s *Supervisor) RunningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, item := range s.processes {
		if item.model.Status == StatusRunning {
			count++
		}
	}
	return count
}

func (s *Supervisor) RuntimeRoot() string { return s.runtimeRoot }

func (s *Supervisor) finishDummy(sessionID string, item *managedProcess) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processes[sessionID] != item {
		close(item.done)
		return
	}
	if item.model.Status == StatusStopping {
		now, code := s.now(), 0
		item.model.Status, item.model.StoppedAt, item.model.ExitCode = StatusStopped, &now, &code
		item.logs.append(formatLog(now, "dummy-runtime", "runtime stopped"))
	}
	close(item.done)
}

func (s *Supervisor) waitTerminal(sessionID string, item *managedProcess) {
	err := item.cmd.Wait()
	code := item.cmd.ProcessState.ExitCode()
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processes[sessionID] != item {
		close(item.done)
		return
	}
	item.model.StoppedAt, item.model.ExitCode = &now, &code
	if item.stopRequested {
		item.model.Status = StatusStopped
		if err != nil {
			item.model.LastError = err.Error()
		}
	} else if code == 0 {
		item.model.Status = StatusExited
	} else {
		item.model.Status, item.model.Crashed = StatusCrashed, true
		if err != nil {
			item.model.LastError = err.Error()
		}
	}
	item.logs.append(formatLog(now, "supervisor", fmt.Sprintf("terminal exited code=%d status=%s", code, item.model.Status)))
	close(item.done)
}

func (s *Supervisor) appendLog(sessionID string, expected *managedProcess, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processes[sessionID] == expected {
		expected.logs.append(formatLog(s.now(), "terminal", line))
	}
}

func (s *Supervisor) resolveWorkingDir(layout SessionRuntimeLayout, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" {
		return layout.WorkDir, nil
	}
	if filepath.IsAbs(relative) {
		return "", errors.New("terminal runtime working directory must be relative to runtime root")
	}
	clean := filepath.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("terminal runtime working directory escapes runtime root")
	}
	resolved := filepath.Join(s.runtimeRoot, clean)
	if !safepath.Within(s.runtimeRoot, resolved) {
		return "", errors.New("terminal runtime working directory escapes runtime root")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect terminal runtime working directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("terminal runtime working directory is not a directory")
	}
	return resolved, nil
}

func trustedEnvironment(profileEnv map[string]string) []string {
	keys := []string{"SystemRoot", "WINDIR", "TEMP", "TMP", "HOME", "USERPROFILE"}
	values := map[string]string{}
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	for key, value := range profileEnv {
		values[key] = value
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func formatLog(at time.Time, source, message string) string {
	return at.Format(time.RFC3339Nano) + " [" + source + "] " + message
}

func safeName(value string) string {
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
	if value == "" {
		return "agent"
	}
	return value
}

func runtimeRelativePath(runtimeRoot, path string) string {
	rel, err := filepath.Rel(runtimeRoot, path)
	if err != nil {
		return path
	}
	return rel
}

func deriveServerJarName(serverCore string) string {
	switch serverCore {
	case "fabric":
		return "fabric-server-launch.jar"
	case "vanilla":
		return "server.jar"
	case "forge":
		return "forge-server.jar"
	default:
		return ""
	}
}

func (s *Supervisor) materializeMCDRRuntime(ctx context.Context, layout SessionRuntimeLayout, request agent.EnvironmentMaterializationRequest, detector pythonRuntimeDetector, manager pythonEnvironmentManager) (map[string]string, error) {
	mcdrLayout, err := layout.MCDR()
	if err != nil {
		return nil, err
	}
	if err := mcdrLayout.Create(); err != nil {
		return nil, fmt.Errorf("prepare MCDR runtime directories: %w", err)
	}
	if err := mcdrLayout.WriteManifest(); err != nil {
		return nil, fmt.Errorf("write MCDR layout manifest: %w", err)
	}
	requestedVersion := "latest"
	if err := writeMCDRUVProject(mcdrLayout.MCDRConfigDir, requestedVersion); err != nil {
		return nil, err
	}
	pythonInstall, err := detector.SelectForMCDR(ctx)
	if err != nil {
		return nil, fmt.Errorf("select Python for MCDR: %w", err)
	}
	managerType := agentpython.ManagerUV
	venvPath := filepath.Join(mcdrLayout.MCDRRoot, "venv")
	venv, err := manager.CreateVenv(ctx, agentpython.VenvRequest{
		SessionID:   request.SessionID,
		VenvPath:    venvPath,
		Python:      pythonInstall,
		ManagerType: managerType,
	})
	if err != nil {
		return nil, fmt.Errorf("create MCDR Python environment with %s: %w", managerType, err)
	}
	proxyURL := os.Getenv("STRATUM_HTTP_PROXY")
	installReq := agentpython.InstallMCDRRequest{
		Venv:        venv,
		Version:     requestedVersion,
		ProxyURL:    proxyURL,
		ManagerType: managerType,
	}
	if installedVersion, verifyErr := manager.VerifyMCDR(ctx, venv); verifyErr == nil {
		return map[string]string{
			"mcdrMaterializationStatus": "ready",
			"mcdrInstallStatus":         "skipped-uv-existing",
			"mcdrLaunchMode":            "trusted-runtime-profile",
			"mcdrUVProjectPath":         filepath.Join(mcdrLayout.MCDRConfigDir, "pyproject.toml"),
			"mcdrUVRunCommand":          "uv run mcdreforged",
			"mcdrRequestedVersion":      requestedVersion,
			"mcdrVersion":               installedVersion,
			"pythonManager":             string(managerType),
			"mcdrEnvironmentPath":       venv.VenvPath,
			"mcdrPythonExecutable":      venv.PythonExec,
			"pythonExecutable":          pythonInstall.ExecutablePath,
			"pythonVersion":             pythonInstall.Version,
			"pythonHasVenv":             fmt.Sprintf("%t", pythonInstall.HasVenv),
			"pythonHasPip":              fmt.Sprintf("%t", pythonInstall.HasPip),
		}, nil
	}
	if err := manager.InstallMCDR(ctx, installReq); err != nil {
		return nil, fmt.Errorf("install MCDR: %w", err)
	}
	installedVersion, err := manager.VerifyMCDR(ctx, venv)
	if err != nil {
		return nil, fmt.Errorf("verify MCDR: %w", err)
	}
	return map[string]string{
		"mcdrMaterializationStatus": "ready",
		"mcdrInstallStatus":         "installed-uv",
		"mcdrLaunchMode":            "trusted-runtime-profile",
		"mcdrUVProjectPath":         filepath.Join(mcdrLayout.MCDRConfigDir, "pyproject.toml"),
		"mcdrUVRunCommand":          "uv run mcdreforged",
		"mcdrRequestedVersion":      requestedVersion,
		"mcdrVersion":               installedVersion,
		"pythonManager":             string(managerType),
		"mcdrEnvironmentPath":       venv.VenvPath,
		"mcdrPythonExecutable":      venv.PythonExec,
		"pythonExecutable":          pythonInstall.ExecutablePath,
		"pythonVersion":             pythonInstall.Version,
		"pythonHasVenv":             fmt.Sprintf("%t", pythonInstall.HasVenv),
		"pythonHasPip":              fmt.Sprintf("%t", pythonInstall.HasPip),
	}, nil
}

func writeMCDRUVProject(configDir, requestedVersion string) error {
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return fmt.Errorf("create MCDR config directory: %w", err)
	}
	dependency := agentpython.MCDRPackageSpec(requestedVersion)
	content := "[project]\n" +
		"name = \"stratum-mcdr-runtime\"\n" +
		"version = \"0.0.0\"\n" +
		"requires-python = \">=3.8\"\n" +
		"dependencies = [\n" +
		fmt.Sprintf("  %q,\n", dependency) +
		"]\n"
	if err := os.WriteFile(filepath.Join(configDir, "pyproject.toml"), []byte(content), 0o640); err != nil {
		return fmt.Errorf("write MCDR uv project: %w", err)
	}
	return nil
}

func (s *Supervisor) materializeJavaAndServerJar(ctx context.Context, layout SessionRuntimeLayout, request agent.EnvironmentMaterializationRequest, detector javaRuntimeDetector, deployer serverJarMaterializer) map[string]string {
	md := map[string]string{}
	if detector == nil || deployer == nil {
		md["javaServerJarStatus"] = "skipped"
		return md
	}
	javaInstall, err := detector.SelectForMinecraftVersion(ctx, request.MinecraftVersion)
	if err != nil {
		md["javaDetectionStatus"] = "failed"
		md["javaDetectionError"] = err.Error()
	} else {
		md["javaDetectionStatus"] = "ok"
		md["javaExecutable"] = javaInstall.ExecutablePath
		md["javaVersion"] = javaInstall.Version
		md["javaMajor"] = fmt.Sprintf("%d", javaInstall.Major)
		md["javaHome"] = javaInstall.Home
	}
	targetDir := layout.WorkDir
	if request.MCDRRequired {
		mcdrLayout, err := layout.MCDR()
		if err == nil {
			targetDir = mcdrLayout.MCDRServerDir
		}
	}
	deployResult, err := deployer.Deploy(ctx, serverjar.DeployRequest{
		SessionID:        request.SessionID,
		ServerCore:       request.ServerCore,
		MinecraftVersion: request.MinecraftVersion,
		LoaderVersion:    request.LoaderVersion,
		TargetDir:        targetDir,
	})
	if err != nil {
		md["serverJarDeployStatus"] = "failed"
		md["serverJarDeployError"] = err.Error()
	} else {
		md["serverJarDeployStatus"] = "ok"
		md["serverJarName"] = deployResult.JarName
		md["serverJarPath"] = deployResult.DeployedPath
		md["serverJarHash"] = deployResult.SHA256
		md["serverJarSize"] = fmt.Sprintf("%d", deployResult.SizeBytes)
		md["serverJarSource"] = deployResult.Source
	}
	return md
}

func (s *Supervisor) MaterializeEnvironment(ctx context.Context, request agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	s.mu.RLock()
	adapter := s.lucyAdapter
	pythonDetector := s.pythonDetector
	pythonManager := s.pythonManager
	javaDetector := s.javaDetector
	serverJarDeployer := s.serverJarDeployer
	s.mu.RUnlock()
	if pythonDetector == nil {
		pythonDetector = agentpython.NewDetector()
	}
	if pythonManager == nil {
		pythonManager = agentpython.NewManager()
	}
	if request.MinecraftVersion == "latest" {
		latest := serverjar.DefaultVersionCache().Latest()
		if latest != "" {
			request.MinecraftVersion = latest
		} else {
			var err error
			latest, err = serverjar.ResolveLatestVersion(ctx)
			if err != nil {
				return agent.EnvironmentMaterializationResult{}, fmt.Errorf("resolve latest Minecraft version: %w", err)
			}
			request.MinecraftVersion = latest
		}
	}
	adapterMode := "unknown"
	switch adapter.(type) {
	case lucy.NoopAdapter:
		adapterMode = "noop"
	case *lucy.CLIAdapter:
		adapterMode = "cli"
	case *lucy.EmbeddedAdapter:
		adapterMode = "embedded"
	}
	layout, err := NewSessionRuntimeLayout(s.runtimeRoot, request.SessionID)
	if err != nil {
		return agent.EnvironmentMaterializationResult{}, err
	}
	if err := layout.Create(); err != nil {
		return agent.EnvironmentMaterializationResult{}, err
	}
	if adapterMode == "embedded" && request.MCDRRequired {
		if mcdrLayout, e := layout.MCDR(); e == nil {
			if ea, ok := adapter.(*lucy.EmbeddedAdapter); ok {
				ea.SetServerDir(mcdrLayout.MCDRServerDir)
			}
		}
	}
	sessionRoot := layout.SessionRoot
	configDir := layout.ConfigDir
	directories := []string{"config", "work", "world", "logs", "mods"}
	for _, dir := range directories {
		dirPath := filepath.Join(sessionRoot, dir)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			return agent.EnvironmentMaterializationResult{}, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	lucyConfigured := adapter != nil && adapterMode != "noop"
	lucyResolutionStatus := "not_requested"
	lucyLockHash := ""
	lucyManifestRuntimePath := ""
	lucyLockRuntimePath := ""
	lucyInstallStatus := ""
	lucyInstalledCount := 0
	lucyFailedCount := 0
	lucyInstallTotalSize := int64(0)
	lucyIntegrityStatus := "not_checked"
	lucyIntegrityOK := true
	lucyIntegrityMissing := []string{}
	lucyIntegrityCorrupt := []string{}
	lucyIntegrityErrors := []string{}
	lucyIntegrityChecked := 0
	var resolvedLock *lucy.EnvironmentLock
	var resolvedSpec *lucy.EnvironmentSpec
	lucyMetadata := map[string]string{}
	serverJarName := deriveServerJarName(request.ServerCore)
	if serverJarName != "" {
		lucyMetadata["serverJarName"] = serverJarName
	}
	if request.MCDRRequired {
		mcdrMetadata, err := s.materializeMCDRRuntime(ctx, layout, request, pythonDetector, pythonManager)
		if err != nil {
			return agent.EnvironmentMaterializationResult{}, err
		}
		for key, value := range mcdrMetadata {
			lucyMetadata[key] = value
		}
	}
	if request.ServerCore != "" && request.ServerCore != "custom" {
		javaServerMetadata := s.materializeJavaAndServerJar(ctx, layout, request, javaDetector, serverJarDeployer)
		for key, value := range javaServerMetadata {
			lucyMetadata[key] = value
		}
	}
	if lucyConfigured {
		lucyResolutionStatus = "resolved"
		lucyManifestPath := filepath.Join(configDir, "lucy.yaml")
		lucyManifestRuntimePath = runtimeRelativePath(s.runtimeRoot, lucyManifestPath)
		selectedManifest, err := selectLucyManifest(ctx, request.LucyManifestRef, request.Packages, lucy.CreateDefault(request.MinecraftVersion, request.LoaderType, request.LoaderVersion, request.MCDRRequired))
		if err != nil {
			lucyResolutionStatus = "failed"
			lucyMetadata["lucyResolutionError"] = err.Error()
			lucyMetadata["lucyResolutionErrorCode"] = string(lucy.ClassifyError(err))
		}
		for key, value := range selectedManifest.metadata {
			lucyMetadata[key] = value
		}
		if lucyResolutionStatus == "failed" {
			// Error metadata has already been recorded above.
		} else if err := writeLucyManifest(ctx, configDir, selectedManifest.manifest); err != nil {
			lucyResolutionStatus = "failed"
			lucyMetadata["lucyResolutionError"] = err.Error()
			lucyMetadata["lucyResolutionErrorCode"] = string(lucy.ClassifyError(err))
		} else {
			lucyMetadata["lucyManifestPath"] = lucyManifestRuntimePath
			spec := lucy.EnvironmentSpec{
				EnvironmentID:    request.EnvironmentID,
				MinecraftVersion: request.MinecraftVersion,
				JavaVersion:      request.JavaVersion,
				LoaderType:       request.LoaderType,
				LoaderVersion:    request.LoaderVersion,
				ServerCore:       request.ServerCore,
				CarpetRequired:   request.CarpetRequired,
				MCDRRequired:     request.MCDRRequired,
				RuntimeProfileID: request.RuntimeProfileID,
				Packages:         selectedManifest.packages,
				LocalArtifacts:   request.LocalArtifacts,
				Metadata: map[string]string{
					"lucyManifestRef": request.LucyManifestRef,
					"lucyLockRef":     request.LucyLockRef,
				},
			}
			resolvedSpec = &spec
			plan, err := adapter.PlanEnvironment(ctx, lucy.PlanEnvironmentRequest{Spec: spec})
			if err != nil {
				lucyResolutionStatus = "failed"
				lucyMetadata["lucyResolutionError"] = err.Error()
				lucyMetadata["lucyResolutionErrorCode"] = string(lucy.ClassifyError(err))
			} else {
				lucyMetadata["lucyPlanActionCount"] = fmt.Sprintf("%d", len(plan.Actions))
				lucyMetadata["lucyPlanWarningCount"] = fmt.Sprintf("%d", len(plan.Warnings))
				lucyMetadata["lucyPlanErrorCount"] = fmt.Sprintf("%d", len(plan.Errors))
				lucyMetadata["lucyPlanRequiresLockUpdate"] = fmt.Sprintf("%t", plan.RequiresLockUpdate)
				lock, err := adapter.LockEnvironment(ctx, lucy.LockEnvironmentRequest{Spec: spec})
				if err != nil {
					lucyResolutionStatus = "failed"
					lucyMetadata["lucyResolutionError"] = err.Error()
					lucyMetadata["lucyResolutionErrorCode"] = string(lucy.ClassifyError(err))
				} else {
					lucyLockPath := filepath.Join(configDir, "lucy-lock.yaml")
					lucyLockRuntimePath = runtimeRelativePath(s.runtimeRoot, lucyLockPath)
					lockJSON, err := json.MarshalIndent(lock, "", "  ")
					if err != nil {
						lucyResolutionStatus = "failed"
						lucyMetadata["lucyResolutionError"] = err.Error()
						lucyMetadata["lucyResolutionErrorCode"] = string(lucy.ClassifyError(err))
					} else if err := os.WriteFile(lucyLockPath, lockJSON, 0o644); err != nil {
						lucyResolutionStatus = "failed"
						lucyMetadata["lucyResolutionError"] = err.Error()
						lucyMetadata["lucyResolutionErrorCode"] = string(lucy.ClassifyError(err))
					} else {
						resolvedLock = &lock
						lucyLockHash = lock.LockHash
						lucyMetadata["lucyLockPath"] = lucyLockRuntimePath
						lucyMetadata["lucyLockHash"] = lucyLockHash
						lucyMetadata["lucyLockPackageCount"] = fmt.Sprintf("%d", len(lock.Packages))
						lucyMetadata["lucyLockArtifactCount"] = fmt.Sprintf("%d", len(lock.Artifacts))
					}
				}
			}
		}
	}
	modsDir := filepath.Join(sessionRoot, "mods")
	if request.MCDRRequired {
		if mcdrLayout, e := layout.MCDR(); e == nil {
			modsDir = filepath.Join(mcdrLayout.MCDRServerDir, "mods")
		}
	}
	if lucyConfigured && lucyResolutionStatus == "resolved" && resolvedLock != nil && len(resolvedLock.Packages) > 0 {
		installResult, err := adapter.InstallPackages(ctx, lucy.InstallPackagesRequest{
			Packages:  resolvedLock.Packages,
			TargetDir: modsDir,
			WorkDir:   configDir,
		})
		if err != nil {
			lucyInstallStatus = "failed"
			lucyMetadata["lucyInstallStatus"] = lucyInstallStatus
			lucyMetadata["lucyInstallError"] = err.Error()
			lucyMetadata["lucyInstallErrorCode"] = string(lucy.ClassifyError(err))
		} else {
			lucyInstallStatus = installResult.Status
			lucyInstalledCount = len(installResult.Installed)
			lucyFailedCount = len(installResult.Failed)
			lucyInstallTotalSize = installResult.TotalSize
			lucyMetadata["lucyInstallStatus"] = lucyInstallStatus
			lucyMetadata["lucyInstalledCount"] = fmt.Sprintf("%d", lucyInstalledCount)
			lucyMetadata["lucyFailedCount"] = fmt.Sprintf("%d", lucyFailedCount)
			lucyMetadata["lucyInstallTotalSize"] = fmt.Sprintf("%d", lucyInstallTotalSize)
			for _, pkg := range installResult.Installed {
				lucyMetadata["lucyInstallPath_"+pkg.ID] = pkg.Path
			}
		}
	}
	if lucyConfigured && lucyResolutionStatus == "resolved" && resolvedLock != nil && len(resolvedLock.Packages) > 0 {
		if resolvedSpec != nil {
			statusResult, err := adapter.CheckStatus(ctx, lucy.StatusRequest{Spec: *resolvedSpec, Lock: resolvedLock})
			if err != nil {
				lucyMetadata["lucyCheckStatusError"] = err.Error()
			} else {
				lucyMetadata["lucyCheckStatusMissing"] = fmt.Sprintf("%d", len(statusResult.Missing))
				lucyMetadata["lucyCheckStatusDrifted"] = fmt.Sprintf("%d", len(statusResult.Drifted))
			}
		}
		integrityResult, err := adapter.VerifyIntegrity(ctx, lucy.IntegrityRequest{
			LockPath: filepath.Join(configDir, "lucy-lock.yaml"),
			ModsDir:  modsDir,
		})
		if err != nil {
			lucyIntegrityStatus = "error"
			lucyIntegrityOK = false
			lucyIntegrityErrors = []string{err.Error()}
			lucyMetadata["lucyIntegrityStatus"] = lucyIntegrityStatus
			lucyMetadata["lucyIntegrityError"] = err.Error()
			lucyMetadata["lucyIntegrityErrorCode"] = string(lucy.ClassifyError(err))
		} else {
			lucyIntegrityStatus = integrityResult.Status
			lucyIntegrityOK = integrityResult.OK
			lucyIntegrityMissing = append([]string(nil), integrityResult.Missing...)
			lucyIntegrityCorrupt = append([]string(nil), integrityResult.Corrupt...)
			lucyIntegrityErrors = append([]string(nil), integrityResult.Errors...)
			lucyIntegrityChecked = integrityResult.Checked
			lucyMetadata["lucyIntegrityStatus"] = lucyIntegrityStatus
			lucyMetadata["lucyIntegrityChecked"] = fmt.Sprintf("%d", integrityResult.Checked)
			lucyMetadata["lucyIntegrityMissing"] = fmt.Sprintf("%d", len(integrityResult.Missing))
			lucyMetadata["lucyIntegrityCorrupt"] = fmt.Sprintf("%d", len(integrityResult.Corrupt))
		}
	}
	if lucyConfigured && lucyResolutionStatus == "resolved" && resolvedLock != nil && len(resolvedLock.Packages) == 0 {
		lucyIntegrityStatus = "ok"
		lucyIntegrityOK = true
		lucyMetadata["lucyIntegrityStatus"] = lucyIntegrityStatus
		lucyMetadata["lucyIntegrityChecked"] = "0"
		lucyMetadata["lucyIntegrityMissing"] = "0"
		lucyMetadata["lucyIntegrityCorrupt"] = "0"
	}
	manifestPath := filepath.Join(configDir, "environment-materialization.json")
	manifest := map[string]interface{}{
		"session_id":               request.SessionID,
		"environment_id":           request.EnvironmentID,
		"environment_name":         request.EnvironmentName,
		"minecraft_version":        request.MinecraftVersion,
		"java_version":             request.JavaVersion,
		"loader_type":              request.LoaderType,
		"loader_version":           request.LoaderVersion,
		"server_core":              request.ServerCore,
		"mcdr_required":            request.MCDRRequired,
		"carpet_required":          request.CarpetRequired,
		"runtime_profile_id":       request.RuntimeProfileID,
		"runtime_profile_required": request.RuntimeProfileRequired,
		"materialized_at":          time.Now().UTC().Format(time.RFC3339),
		"status":                   "prepared",
		"prepared_directories":     directories,
		"lucy_adapter_configured":  lucyConfigured,
		"lucy_adapter_mode":        adapterMode,
		"lucy_resolution_status":   lucyResolutionStatus,
		"lucyResolutionStatus":     lucyResolutionStatus,
		"lucyLockHash":             lucyLockHash,
		"lucyManifestPath":         lucyManifestRuntimePath,
		"lucyLockPath":             lucyLockRuntimePath,
		"lucyInstallStatus":        lucyInstallStatus,
		"lucyInstalledCount":       lucyInstalledCount,
		"lucyFailedCount":          lucyFailedCount,
		"lucyInstallTotalSize":     lucyInstallTotalSize,
		"lucyIntegrityStatus":      lucyIntegrityStatus,
		"lucyIntegrityChecked":     lucyIntegrityChecked,
		"lucyIntegrityMissing":     len(lucyIntegrityMissing),
		"lucyIntegrityCorrupt":     len(lucyIntegrityCorrupt),
		"notes":                    "Environment materialization prepared directories only; it did not install Java, Minecraft, Fabric, Carpet, Lucy, MCDR, or start any runtime.",
	}
	for key, value := range lucyMetadata {
		manifest[key] = value
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return agent.EnvironmentMaterializationResult{}, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestJSON, 0o644); err != nil {
		return agent.EnvironmentMaterializationResult{}, fmt.Errorf("write manifest: %w", err)
	}
	metadata := map[string]string{
		"manifestPath":          manifestPath,
		"sessionRoot":           sessionRoot,
		"lucyAdapterMode":       adapterMode,
		"lucyResolutionStatus":  lucyResolutionStatus,
		"lucyAdapterConfigured": fmt.Sprintf("%t", lucyConfigured),
	}
	for key, value := range lucyMetadata {
		metadata[key] = value
	}
	result := agent.EnvironmentMaterializationResult{
		SessionID:              request.SessionID,
		EnvironmentID:          request.EnvironmentID,
		EnvironmentName:        request.EnvironmentName,
		MinecraftVersion:       request.MinecraftVersion,
		JavaVersion:            request.JavaVersion,
		LoaderType:             request.LoaderType,
		LoaderVersion:          request.LoaderVersion,
		ServerCore:             request.ServerCore,
		MCDRRequired:           request.MCDRRequired,
		CarpetRequired:         request.CarpetRequired,
		RuntimeProfileID:       request.RuntimeProfileID,
		RuntimeProfileRequired: request.RuntimeProfileRequired,
		LucyResolutionStatus:   lucyResolutionStatus,
		LucyLockHash:           lucyLockHash,
		LucyManifestPath:       lucyManifestRuntimePath,
		LucyLockPath:           lucyLockRuntimePath,
		LucyInstallStatus:      lucyInstallStatus,
		LucyInstalledCount:     lucyInstalledCount,
		LucyFailedCount:        lucyFailedCount,
		LucyInstallTotalSize:   lucyInstallTotalSize,
		LucyIntegrityStatus:    lucyIntegrityStatus,
		MaterializedAt:         time.Now().UTC(),
		Status:                 "prepared",
		Directories:            directories,
		Metadata:               metadata,
	}
	if !lucyIntegrityOK && lucyConfigured {
		return agent.EnvironmentMaterializationResult{}, agent.NewEnvironmentIntegrityError(request.SessionID, lucyIntegrityStatus, lucyIntegrityMissing, lucyIntegrityCorrupt, lucyIntegrityErrors)
	}
	return result, nil
}

func (s *Supervisor) GetSessionRuntimeStatus(ctx context.Context, sessionID string) (agent.SessionRuntimeStatus, error) {
	sessionRoot := filepath.Join(s.runtimeRoot, "sessions", safeName(sessionID))
	status := agent.SessionRuntimeStatus{
		SessionID: sessionID,
		CheckedAt: s.now(),
	}
	if info, err := os.Stat(s.runtimeRoot); err == nil && info.IsDir() {
		status.RuntimeRootExists = true
	}
	if info, err := os.Stat(sessionRoot); err == nil && info.IsDir() {
		status.SessionRootExists = true
	}
	checkDir := func(name string) bool {
		path := filepath.Join(sessionRoot, name)
		info, err := os.Stat(path)
		return err == nil && info.IsDir()
	}
	status.WorkDirExists = checkDir("work")
	status.ConfigDirExists = checkDir("config")
	status.LogsDirExists = checkDir("logs")
	status.ArtifactsDirExists = checkDir("artifacts")
	status.CheckpointsDirExists = checkDir("checkpoints")
	status.TmpDirExists = checkDir("tmp")
	envManifestPath := filepath.Join(sessionRoot, "config", "environment-materialization.json")
	if _, err := os.Stat(envManifestPath); err == nil {
		relPath, _ := filepath.Rel(s.runtimeRoot, envManifestPath)
		envStatus := &agent.EnvironmentManifestStatus{
			Exists:              true,
			Path:                envManifestPath,
			RuntimeRelativePath: relPath,
		}
		if data, err := os.ReadFile(envManifestPath); err == nil {
			var manifest map[string]interface{}
			if decodeErr := json.Unmarshal(data, &manifest); decodeErr == nil {
				if v, ok := manifest["status"].(string); ok {
					envStatus.Status = v
				}
				if v, ok := manifest["environment_id"].(string); ok {
					envStatus.EnvironmentID = v
				}
				if v, ok := manifest["minecraft_version"].(string); ok {
					envStatus.MinecraftVersion = v
				}
				if v, ok := manifest["loader_type"].(string); ok {
					envStatus.LoaderType = v
				}
				if v, ok := manifest["server_core"].(string); ok {
					envStatus.ServerCore = v
				}
				if v, ok := manifest["runtime_profile_id"].(string); ok {
					envStatus.RuntimeProfileID = v
				}
				if v, ok := manifest["mcdr_required"].(bool); ok {
					envStatus.MCDRRequired = v
				}
				if v, ok := manifest["lucyLockHash"].(string); ok {
					envStatus.LucyLockHash = v
				}
			} else {
				envStatus.ErrorMessage = decodeErr.Error()
			}
		} else {
			envStatus.ErrorMessage = err.Error()
		}
		status.EnvironmentManifest = envStatus
	}
	mcdrRoot := filepath.Join(sessionRoot, "work", "mcdr")
	mcdrManifestPath := filepath.Join(mcdrRoot, "mcdr-layout.json")
	if info, err := os.Stat(mcdrRoot); err == nil && info.IsDir() {
		mcdrStatus := &agent.MCDRLayoutStatus{
			MCDRRootExists: true,
		}
		if _, err := os.Stat(mcdrManifestPath); err == nil {
			mcdrStatus.ManifestExists = true
			mcdrStatus.ManifestPath = mcdrManifestPath
			if relPath, err := filepath.Rel(s.runtimeRoot, mcdrManifestPath); err == nil {
				mcdrStatus.RuntimeRelativePath = relPath
			}
		}
		status.MCDRLayout = mcdrStatus
	}
	materializedManifestPath := filepath.Join(sessionRoot, "artifacts", "staged-artifacts.json")
	if _, err := os.Stat(materializedManifestPath); err == nil {
		relPath, _ := filepath.Rel(s.runtimeRoot, materializedManifestPath)
		matStatus := &agent.MaterializedArtifactsStatus{
			ManifestExists:      true,
			ManifestPath:        materializedManifestPath,
			RuntimeRelativePath: relPath,
		}
		if data, err := os.ReadFile(materializedManifestPath); err == nil {
			var manifest StagingManifest
			if json.Unmarshal(data, &manifest) == nil {
				matStatus.Count = len(manifest.Items)
			}
		}
		status.MaterializedArtifacts = matStatus
	}
	appliedManifestPath := filepath.Join(sessionRoot, "artifacts", "applied-artifacts.json")
	if _, err := os.Stat(appliedManifestPath); err == nil {
		relPath, _ := filepath.Rel(s.runtimeRoot, appliedManifestPath)
		appStatus := &agent.AppliedArtifactsStatus{
			ManifestExists:      true,
			ManifestPath:        appliedManifestPath,
			RuntimeRelativePath: relPath,
		}
		if data, err := os.ReadFile(appliedManifestPath); err == nil {
			var records map[string]interface{}
			if json.Unmarshal(data, &records) == nil {
				if items, ok := records["records"].([]interface{}); ok {
					appStatus.Count = len(items)
				}
			}
		}
		status.AppliedArtifacts = appStatus
	}
	processModel := s.InspectProcess(sessionID)
	if processModel.Status != StatusNotStarted {
		status.ProcessStatus = &agent.ProcessStatusSummary{
			Status:           string(processModel.Status),
			RuntimeProfileID: processModel.RuntimeProfileID,
			PID:              processModel.PID,
			Crashed:          processModel.Crashed,
			StartedAt:        processModel.StartedAt,
			StoppedAt:        processModel.StoppedAt,
		}
	}
	serverPropsPath := filepath.Join(sessionRoot, "work", "server.properties")
	if data, err := os.ReadFile(serverPropsPath); err == nil {
		if cfg, err := serverproperties.Parse(bytes.NewReader(data)); err == nil {
			if snapshot := serverproperties.ToWorldProfileSnapshot(cfg, ""); snapshot != nil {
				status.WorldProfile = &agent.WorldProfileStatus{
					Seed:               snapshot.Seed,
					LevelType:          snapshot.LevelType,
					GeneratorSettings:  snapshot.GeneratorSettings,
					GenerateStructures: snapshot.GenerateStructures,
					SpawnRadius:        snapshot.SpawnRadius,
					Difficulty:         snapshot.Difficulty,
					ViewDistance:       snapshot.ViewDistance,
				}
			}
		}
	}
	return status, nil
}

func signalByName(name string) os.Signal {
	switch name {
	case "SIGTERM":
		return syscall.SIGTERM
	case "SIGINT":
		return syscall.SIGINT
	case "SIGKILL":
		return syscall.SIGKILL
	default:
		return nil
	}
}
