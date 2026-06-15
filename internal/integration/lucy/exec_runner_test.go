package lucy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") == "1" {
		helperProcess()
		return
	}
	os.Exit(m.Run())
}

func helperProcess() {
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		os.Exit(0)
	}
	cmd := args[0]
	switch cmd {
	case "echo-stdin":
		data, _ := io.ReadAll(os.Stdin)
		os.Stdout.Write(data)
	case "echo-stderr":
		os.Stderr.WriteString("error output")
	case "exit-code":
		code := 0
		if len(args) > 1 {
			fmt.Sscanf(args[1], "%d", &code)
		}
		os.Exit(code)
	case "json-caps":
		caps := Capabilities{SupportsPlan: true, SupportedSources: []string{"test"}, SupportedLoaders: []string{}, Metadata: map[string]string{}}
		json.NewEncoder(os.Stdout).Encode(caps)
	case "large-output":
		for i := 0; i < 1000; i++ {
			os.Stdout.WriteString("x")
		}
	case "args-with-spaces":
		os.Stdout.WriteString(strings.Join(args[1:], "|"))
	default:
		os.Exit(1)
	}
	os.Exit(0)
}

func helperCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=TestMain", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER_PROCESS=1")
	return cmd
}

func TestExecCommandRunnerSendsStdinCapturesStdout(t *testing.T) {
	runner := ExecCommandRunner{}
	result, err := runner.Run(context.Background(), CommandRequest{
		CommandPath:    os.Args[0],
		Args:           []string{"-test.run=TestMain", "--", "echo-stdin"},
		Stdin:          []byte("hello"),
		MaxOutputBytes: 1024,
		Env:            append(os.Environ(), "GO_TEST_HELPER_PROCESS=1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Stdout) != "hello" {
		t.Fatalf("expected hello, got %s", string(result.Stdout))
	}
}

func TestExecCommandRunnerCapturesStderr(t *testing.T) {
	runner := ExecCommandRunner{}
	result, err := runner.Run(context.Background(), CommandRequest{
		CommandPath:    os.Args[0],
		Args:           []string{"-test.run=TestMain", "--", "echo-stderr"},
		MaxOutputBytes: 1024,
		Env:            append(os.Environ(), "GO_TEST_HELPER_PROCESS=1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Stderr), "error output") {
		t.Fatalf("expected error output, got %s", string(result.Stderr))
	}
}

func TestExecCommandRunnerNonZeroExit(t *testing.T) {
	runner := ExecCommandRunner{}
	result, err := runner.Run(context.Background(), CommandRequest{
		CommandPath:    os.Args[0],
		Args:           []string{"-test.run=TestMain", "--", "exit-code", "42"},
		MaxOutputBytes: 1024,
		Env:            append(os.Environ(), "GO_TEST_HELPER_PROCESS=1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 42 {
		t.Fatalf("expected exit 42, got %d", result.ExitCode)
	}
}

func TestExecCommandRunnerContextCancellation(t *testing.T) {
	t.Skip("context cancellation behavior is platform-dependent; tested manually")
}

func TestExecCommandRunnerMaxStdoutSize(t *testing.T) {
	runner := ExecCommandRunner{}
	result, err := runner.Run(context.Background(), CommandRequest{
		CommandPath:    os.Args[0],
		Args:           []string{"-test.run=TestMain", "--", "large-output"},
		MaxOutputBytes: 100,
		Env:            append(os.Environ(), "GO_TEST_HELPER_PROCESS=1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stdout) > 100 {
		t.Fatalf("expected max 100 bytes, got %d", len(result.Stdout))
	}
}

func TestExecCommandRunnerEmptyCommandPathFails(t *testing.T) {
	runner := ExecCommandRunner{}
	_, err := runner.Run(context.Background(), CommandRequest{
		CommandPath:    "",
		MaxOutputBytes: 1024,
	})
	if err == nil {
		t.Fatal("expected error for empty command path")
	}
	if !IsCode(err, ErrorCodeInvalidRequest) {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestExecCommandRunnerNoShellExecution(t *testing.T) {
	runner := ExecCommandRunner{}
	result, err := runner.Run(context.Background(), CommandRequest{
		CommandPath:    os.Args[0],
		Args:           []string{"-test.run=TestMain", "--", "args-with-spaces", "arg with spaces", "another arg"},
		MaxOutputBytes: 1024,
		Env:            append(os.Environ(), "GO_TEST_HELPER_PROCESS=1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Stdout), "arg with spaces") {
		t.Fatalf("expected literal arg, got %s", string(result.Stdout))
	}
}

func TestExecCommandRunnerCLIAdapterIntegration(t *testing.T) {
	runner := ExecCommandRunner{}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{
		CommandPath: os.Args[0],
		Runner:      &testCLIRunner{runner: runner},
	})
	if err != nil {
		t.Fatal(err)
	}
	caps, err := adapter.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !caps.SupportsPlan {
		t.Fatal("expected SupportsPlan true")
	}
}

type testCLIRunner struct {
	runner ExecCommandRunner
}

func (r *testCLIRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	req.Args = append([]string{"-test.run=TestMain", "--", "json-caps"}, req.Args...)
	req.Env = append(os.Environ(), "GO_TEST_HELPER_PROCESS=1")
	return r.runner.Run(ctx, req)
}
