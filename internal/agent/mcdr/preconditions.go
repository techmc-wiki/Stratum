package mcdr

import (
	"context"
	"fmt"
	"os"
	"strings"

	agentjava "github.com/stratummc/stratum/internal/agent/java"
	agentpython "github.com/stratummc/stratum/internal/agent/python"
)

type PreconditionStatus string

const (
	PreconditionOK      PreconditionStatus = "ok"
	PreconditionMissing PreconditionStatus = "missing"
	PreconditionInvalid PreconditionStatus = "invalid"
	PreconditionSkipped PreconditionStatus = "skipped"
)

type PreconditionCheck struct {
	Name    string             `json:"name"`
	Status  PreconditionStatus `json:"status"`
	Message string             `json:"message,omitempty"`
	Path    string             `json:"path,omitempty"`
}

type PreconditionResult struct {
	Ready  bool                `json:"ready"`
	Checks []PreconditionCheck `json:"checks"`
}

type PreconditionRequest struct {
	SessionID        string
	MinecraftVersion string
	JavaExecutable   string
	MCDRExecutable   string
	ServerJarPath    string
	EULAPath         string
	RequireEULA      bool
}

type PreconditionChecker struct {
	Stat           func(string) (os.FileInfo, error)
	ReadFile       func(string) ([]byte, error)
	RunCommand     agentpython.CommandRunner
	JavaDetector   *agentjava.Detector
	PythonDetector *agentpython.Detector
}

func NewPreconditionChecker() *PreconditionChecker {
	return &PreconditionChecker{
		Stat:           os.Stat,
		ReadFile:       os.ReadFile,
		RunCommand:     runCommand,
		JavaDetector:   agentjava.NewDetector(),
		PythonDetector: agentpython.NewDetector(),
	}
}

func (c *PreconditionChecker) Check(ctx context.Context, req PreconditionRequest) PreconditionResult {
	checker := c.normalized()
	result := PreconditionResult{Ready: true}
	result.add(checkSessionID(req.SessionID))
	result.add(checkJava(ctx, checker.JavaDetector, req.MinecraftVersion, req.JavaExecutable))
	result.add(checkMCDR(ctx, checker.RunCommand, req.MCDRExecutable))
	result.add(checkFile(checker.Stat, "server_jar", req.ServerJarPath, true))
	if req.RequireEULA || strings.TrimSpace(req.EULAPath) != "" {
		result.add(checkEULA(checker.Stat, checker.ReadFile, req.EULAPath))
	} else {
		result.add(PreconditionCheck{Name: "eula", Status: PreconditionSkipped, Message: "EULA check not requested"})
	}
	result.Ready = result.allOKOrSkipped()
	return result
}

func (r *PreconditionResult) add(check PreconditionCheck) {
	r.Checks = append(r.Checks, check)
}

func (r PreconditionResult) allOKOrSkipped() bool {
	for _, check := range r.Checks {
		if check.Status != PreconditionOK && check.Status != PreconditionSkipped {
			return false
		}
	}
	return true
}

func (c *PreconditionChecker) normalized() *PreconditionChecker {
	if c == nil {
		return NewPreconditionChecker()
	}
	copy := *c
	if copy.Stat == nil {
		copy.Stat = os.Stat
	}
	if copy.ReadFile == nil {
		copy.ReadFile = os.ReadFile
	}
	if copy.RunCommand == nil {
		copy.RunCommand = runCommand
	}
	if copy.JavaDetector == nil {
		copy.JavaDetector = agentjava.NewDetector()
	}
	if copy.PythonDetector == nil {
		copy.PythonDetector = agentpython.NewDetector()
	}
	return &copy
}

func checkSessionID(sessionID string) PreconditionCheck {
	if strings.TrimSpace(sessionID) == "" {
		return PreconditionCheck{Name: "session", Status: PreconditionInvalid, Message: "session id is required"}
	}
	return PreconditionCheck{Name: "session", Status: PreconditionOK, Message: "session id present"}
}

func checkJava(ctx context.Context, detector *agentjava.Detector, minecraftVersion, executable string) PreconditionCheck {
	required, err := agentjava.RequiredMajorForMinecraftVersion(minecraftVersion)
	if err != nil {
		return PreconditionCheck{Name: "java", Status: PreconditionInvalid, Message: err.Error(), Path: executable}
	}
	if strings.TrimSpace(executable) == "" {
		selected, err := detector.SelectForVersion(ctx, required)
		if err != nil {
			return PreconditionCheck{Name: "java", Status: PreconditionMissing, Message: err.Error()}
		}
		return PreconditionCheck{Name: "java", Status: PreconditionOK, Message: fmt.Sprintf("selected Java %d for Minecraft %s", selected.Major, minecraftVersion), Path: selected.ExecutablePath}
	}
	output, err := detector.RunVersion(ctx, executable)
	if err != nil {
		return PreconditionCheck{Name: "java", Status: PreconditionInvalid, Message: err.Error(), Path: executable}
	}
	_, major, err := agentjava.ParseVersionOutput(output)
	if err != nil {
		return PreconditionCheck{Name: "java", Status: PreconditionInvalid, Message: err.Error(), Path: executable}
	}
	if major < required {
		return PreconditionCheck{Name: "java", Status: PreconditionInvalid, Message: fmt.Sprintf("Java %d does not satisfy Minecraft %s requirement Java %d", major, minecraftVersion, required), Path: executable}
	}
	return PreconditionCheck{Name: "java", Status: PreconditionOK, Message: fmt.Sprintf("Java %d satisfies Minecraft %s", major, minecraftVersion), Path: executable}
}

func checkMCDR(ctx context.Context, run agentpython.CommandRunner, executable string) PreconditionCheck {
	if strings.TrimSpace(executable) == "" {
		return PreconditionCheck{Name: "mcdr", Status: PreconditionMissing, Message: "MCDR executable path is required"}
	}
	output, err := run(ctx, executable, "--version")
	if err != nil {
		return PreconditionCheck{Name: "mcdr", Status: PreconditionInvalid, Message: err.Error(), Path: executable}
	}
	message := strings.TrimSpace(output)
	if message == "" {
		message = "MCDR executable responded"
	}
	return PreconditionCheck{Name: "mcdr", Status: PreconditionOK, Message: message, Path: executable}
}

func checkFile(stat func(string) (os.FileInfo, error), name, path string, required bool) PreconditionCheck {
	if strings.TrimSpace(path) == "" {
		if required {
			return PreconditionCheck{Name: name, Status: PreconditionMissing, Message: name + " path is required"}
		}
		return PreconditionCheck{Name: name, Status: PreconditionSkipped, Message: name + " check not requested"}
	}
	info, err := stat(path)
	if err != nil {
		return PreconditionCheck{Name: name, Status: PreconditionMissing, Message: err.Error(), Path: path}
	}
	if info.IsDir() {
		return PreconditionCheck{Name: name, Status: PreconditionInvalid, Message: "path is a directory", Path: path}
	}
	return PreconditionCheck{Name: name, Status: PreconditionOK, Message: "file exists", Path: path}
}

func checkEULA(stat func(string) (os.FileInfo, error), readFile func(string) ([]byte, error), path string) PreconditionCheck {
	fileCheck := checkFile(stat, "eula", path, true)
	if fileCheck.Status != PreconditionOK {
		return fileCheck
	}
	data, err := readFile(path)
	if err != nil {
		return PreconditionCheck{Name: "eula", Status: PreconditionInvalid, Message: err.Error(), Path: path}
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.EqualFold(line, "eula=true") {
			return PreconditionCheck{Name: "eula", Status: PreconditionOK, Message: "EULA accepted", Path: path}
		}
	}
	return PreconditionCheck{Name: "eula", Status: PreconditionInvalid, Message: "eula=true is required", Path: path}
}

func runCommand(ctx context.Context, path string, args ...string) (string, error) {
	return agentpython.NewManager().Run(ctx, path, args...)
}
