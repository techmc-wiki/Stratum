package lucy

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if req.CommandPath == "" {
		return CommandResult{}, NewAdapterError(ErrorCodeInvalidRequest, "command path required", nil, false)
	}
	cmd := exec.CommandContext(ctx, req.CommandPath, req.Args...)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}
	if len(req.Env) > 0 {
		cmd.Env = req.Env
	}
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{w: &stdout, limit: req.MaxOutputBytes}
	cmd.Stderr = &limitWriter{w: &stderr, limit: req.MaxOutputBytes}
	err := cmd.Run()
	result := CommandResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: 0,
		TimedOut: false,
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		} else if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			result.TimedOut = ctx.Err() == context.DeadlineExceeded
			return result, err
		} else {
			return result, NewAdapterError(ErrorCodeIOError, "failed to run command", err, false)
		}
	}
	return result, nil
}

type limitWriter struct {
	w     io.Writer
	limit int64
	n     int64
}

func (lw *limitWriter) Write(p []byte) (int, error) {
	if lw.n >= lw.limit {
		return len(p), nil
	}
	n := len(p)
	if lw.n+int64(n) > lw.limit {
		n = int(lw.limit - lw.n)
	}
	written, err := lw.w.Write(p[:n])
	lw.n += int64(written)
	if err != nil {
		return written, err
	}
	return len(p), nil
}
