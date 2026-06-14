package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
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

const RuntimeModeDummy = "dummy-process"
const RuntimeModeTerminal = "managed-terminal"
const defaultLogBytes = 256 * 1024

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
	mu          sync.RWMutex
	agentID     string
	runtimeRoot string
	now         func() time.Time
	processes   map[string]*managedProcess
	sequence    uint64
	maxLogBytes int
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
	return &Supervisor{agentID: agentID, runtimeRoot: filepath.Clean(root), now: func() time.Time { return time.Now().UTC() }, processes: map[string]*managedProcess{}, maxLogBytes: maxLogBytes}, nil
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
	if profile.RuntimeType == runtimeprofile.TypeTerminal {
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
	if profile.RuntimeType == runtimeprofile.TypeTerminal {
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
	if item.profile.StopStrategy == runtimeprofile.StopStdin {
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
	if !pathWithin(s.runtimeRoot, resolved) {
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

func (s *Supervisor) MaterializeEnvironment(ctx context.Context, request agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	sessionRoot := filepath.Join(s.runtimeRoot, "sessions", safeName(request.SessionID))
	configDir := filepath.Join(sessionRoot, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return agent.EnvironmentMaterializationResult{}, fmt.Errorf("create config directory: %w", err)
	}
	manifestPath := filepath.Join(configDir, "environment-materialization.json")
	directories := []string{"config", "world", "logs", "mods"}
	for _, dir := range directories {
		dirPath := filepath.Join(sessionRoot, dir)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			return agent.EnvironmentMaterializationResult{}, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	metadata := map[string]string{
		"manifestPath": manifestPath,
		"sessionRoot":  sessionRoot,
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
		MaterializedAt:         time.Now().UTC(),
		Status:                 "prepared",
		Directories:            directories,
		Metadata:               metadata,
	}
	return result, nil
}
