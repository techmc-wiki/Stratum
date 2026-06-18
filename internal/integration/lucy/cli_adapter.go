package lucy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

type CommandRequest struct {
	CommandPath    string
	Args           []string
	Stdin          []byte
	WorkingDir     string
	Env            []string
	MaxOutputBytes int64
}

type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	TimedOut bool
}

type CLIAdapterOptions struct {
	CommandPath    string
	Timeout        time.Duration
	WorkingDir     string
	Env            []string
	MaxOutputBytes int64
	Runner         CommandRunner
	UseExec        bool
}

type CLIAdapter struct {
	commandPath    string
	timeout        time.Duration
	workingDir     string
	env            []string
	maxOutputBytes int64
	runner         CommandRunner
}

var _ Adapter = (*CLIAdapter)(nil)

func NewCLIAdapter(opts CLIAdapterOptions) (*CLIAdapter, error) {
	if opts.CommandPath == "" {
		return nil, NewAdapterError(ErrorCodeInvalidRequest, "command path required", nil, false)
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxOutputBytes == 0 {
		opts.MaxOutputBytes = 10 * 1024 * 1024
	}
	if opts.Runner == nil {
		if opts.UseExec {
			opts.Runner = ExecCommandRunner{}
		} else {
			return nil, NewAdapterError(ErrorCodeInvalidRequest, "runner required", nil, false)
		}
	}
	return &CLIAdapter{
		commandPath:    opts.CommandPath,
		timeout:        opts.Timeout,
		workingDir:     opts.WorkingDir,
		env:            opts.Env,
		maxOutputBytes: opts.MaxOutputBytes,
		runner:         opts.Runner,
	}, nil
}

func (a *CLIAdapter) Capabilities(ctx context.Context) (Capabilities, error) {
	request := struct{}{}
	var response Capabilities
	if err := a.run(ctx, "capabilities", request, &response); err != nil {
		return Capabilities{}, err
	}
	if err := response.Validate(); err != nil {
		return Capabilities{}, NewAdapterError(ErrorCodeValidationFailed, "invalid capabilities response", err, false)
	}
	return response, nil
}

func (a *CLIAdapter) PlanEnvironment(ctx context.Context, req PlanEnvironmentRequest) (EnvironmentPlan, error) {
	if err := req.Spec.Validate(); err != nil {
		return EnvironmentPlan{}, NewAdapterError(ErrorCodeValidationFailed, "invalid plan request", err, false)
	}
	var response EnvironmentPlan
	if err := a.run(ctx, "plan_environment", req, &response); err != nil {
		return EnvironmentPlan{}, err
	}
	if err := response.Validate(); err != nil {
		return EnvironmentPlan{}, NewAdapterError(ErrorCodeValidationFailed, "invalid plan response", err, false)
	}
	return response, nil
}

func (a *CLIAdapter) LockEnvironment(ctx context.Context, req LockEnvironmentRequest) (EnvironmentLock, error) {
	if err := req.Spec.Validate(); err != nil {
		return EnvironmentLock{}, NewAdapterError(ErrorCodeValidationFailed, "invalid lock request", err, false)
	}
	var response EnvironmentLock
	if err := a.run(ctx, "lock_environment", req, &response); err != nil {
		return EnvironmentLock{}, err
	}
	if err := response.Validate(); err != nil {
		return EnvironmentLock{}, NewAdapterError(ErrorCodeValidationFailed, "invalid lock response", err, false)
	}
	return response, nil
}

func (a *CLIAdapter) CheckStatus(ctx context.Context, req StatusRequest) (EnvironmentStatus, error) {
	if err := req.Spec.Validate(); err != nil {
		return EnvironmentStatus{}, NewAdapterError(ErrorCodeValidationFailed, "invalid status request", err, false)
	}
	var response EnvironmentStatus
	if err := a.run(ctx, "check_status", req, &response); err != nil {
		return EnvironmentStatus{}, err
	}
	return response, nil
}

func (a *CLIAdapter) InstallPackages(ctx context.Context, req InstallPackagesRequest) (InstallPackagesResult, error) {
	if err := req.Validate(); err != nil {
		return InstallPackagesResult{}, NewAdapterError(ErrorCodeValidationFailed, "invalid install request", err, false)
	}
	var response InstallPackagesResult
	if err := a.runWithWorkingDir(ctx, "install", []string{"install", "--json"}, req.WorkDir, req, &response); err != nil {
		return InstallPackagesResult{}, err
	}
	fillInstallPaths(req, &response)
	response = validateInstalledHashes(req, response)
	if err := response.Validate(); err != nil {
		return InstallPackagesResult{}, NewAdapterError(ErrorCodeValidationFailed, "invalid install response", err, false)
	}
	return response, nil
}

func (a *CLIAdapter) VerifyIntegrity(ctx context.Context, req IntegrityRequest) (IntegrityResult, error) {
	if err := req.Validate(); err != nil {
		return IntegrityResult{}, NewAdapterError(ErrorCodeValidationFailed, "invalid integrity request", err, false)
	}
	result, err := NewProbeService(req.ModsDir).VerifyIntegrityFromLock(ctx, req.LockPath, req.ModsDir)
	if err != nil {
		return IntegrityResult{}, NewAdapterError(ErrorCodeIOError, "verify lock integrity", err, false)
	}
	if err := result.Validate(); err != nil {
		return IntegrityResult{}, NewAdapterError(ErrorCodeValidationFailed, "invalid integrity result", err, false)
	}
	return result, nil
}

func (a *CLIAdapter) run(ctx context.Context, operation string, request, response interface{}) error {
	return a.runWithWorkingDir(ctx, operation, []string{operation, "--json"}, a.workingDir, request, response)
}

func (a *CLIAdapter) runWithWorkingDir(ctx context.Context, operation string, args []string, workingDir string, request, response interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	stdin, err := json.Marshal(request)
	if err != nil {
		return NewAdapterError(ErrorCodeInternalError, fmt.Sprintf("failed to marshal %s request", operation), err, false)
	}
	result, err := a.runner.Run(ctx, CommandRequest{
		CommandPath:    a.commandPath,
		Args:           args,
		Stdin:          stdin,
		WorkingDir:     workingDir,
		Env:            a.env,
		MaxOutputBytes: a.maxOutputBytes,
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return NewAdapterError(ErrorCodeTimeout, fmt.Sprintf("%s timed out", operation), err, true)
		}
		if ctx.Err() == context.Canceled {
			return NewAdapterError(ErrorCodeCancelled, fmt.Sprintf("%s cancelled", operation), err, true)
		}
		return NewAdapterError(ErrorCodeIOError, fmt.Sprintf("%s runner failed", operation), err, true)
	}
	if result.TimedOut {
		return NewAdapterError(ErrorCodeTimeout, fmt.Sprintf("%s timed out", operation), nil, true)
	}
	if result.ExitCode != 0 {
		return NewAdapterError(ErrorCodeExternalCommandFailed, fmt.Sprintf("%s exited %d: %s", operation, result.ExitCode, string(result.Stderr)), nil, false)
	}
	if int64(len(result.Stdout)) > a.maxOutputBytes {
		return NewAdapterError(ErrorCodeIOError, fmt.Sprintf("%s output exceeds %d bytes", operation, a.maxOutputBytes), nil, false)
	}
	if err := json.Unmarshal(result.Stdout, response); err != nil {
		return NewAdapterError(ErrorCodeInternalError, fmt.Sprintf("%s returned invalid JSON", operation), err, false)
	}
	return nil
}
