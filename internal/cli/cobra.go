package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

type commandRuntime struct {
	stdout           io.Writer
	stderr           io.Writer
	dataDirectory    string
	artifactBlobRoot string
	agentURL         string
	agentToken       string
	agentTimeout     time.Duration
	agentLocal       bool
	store            *filesystem.Store
	agentClient      agent.AgentClient
	agentMode        string
}

func runCobra(args []string, stdout, stderr io.Writer) int {
	runtime := &commandRuntime{stdout: stdout, stderr: stderr}
	remaining, code := runtime.parseGlobalFlags(args)
	if code != 0 {
		return code
	}
	root := newRootCommand(runtime)
	root.SetContext(runtime.context())
	root.SetArgs(remaining)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		if code, ok := err.(exitCodeError); ok {
			return int(code)
		}
		fmt.Fprintln(stderr, err)
		return 2
	}
	return 0
}

func (r *commandRuntime) parseGlobalFlags(args []string) ([]string, int) {
	r.dataDirectory = defaultDataDirectory
	r.artifactBlobRoot = defaultArtifactBlobRoot
	r.agentTimeout = 10 * time.Minute
	remaining := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case "--data-dir", "--artifact-blob-root", "--agent-url", "--agent-token", "--agent-timeout":
			if !hasValue {
				index++
				if index >= len(args) {
					fmt.Fprintf(r.stderr, "%s requires a value\n", name)
					return nil, 2
				}
				value = args[index]
			}
			if code := r.setGlobalFlag(name, value); code != 0 {
				return nil, code
			}
		case "--agent-local":
			if !hasValue {
				value = "true"
			}
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				fmt.Fprintf(r.stderr, "%s must be a boolean\n", name)
				return nil, 2
			}
			r.agentLocal = enabled
		default:
			remaining = append(remaining, arg)
		}
	}
	return remaining, 0
}

func (r *commandRuntime) setGlobalFlag(name, value string) int {
	switch name {
	case "--data-dir":
		r.dataDirectory = value
	case "--artifact-blob-root":
		r.artifactBlobRoot = value
	case "--agent-url":
		r.agentURL = value
	case "--agent-token":
		r.agentToken = value
	case "--agent-timeout":
		timeout, err := time.ParseDuration(value)
		if err != nil {
			fmt.Fprintf(r.stderr, "parse --agent-timeout: %v\n", err)
			return 2
		}
		r.agentTimeout = timeout
	}
	return 0
}

type exitCodeError int

func (e exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e) }

type commandHandler func(context.Context, *commandRuntime, []string) int

func newRootCommand(runtime *commandRuntime) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "stratum",
		Short:         "StratumMC collaborative testing control plane CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().String("data-dir", runtime.dataDirectory, "metadata data directory")
	cmd.PersistentFlags().String("artifact-blob-root", runtime.artifactBlobRoot, "artifact blob storage root")
	cmd.PersistentFlags().String("agent-url", runtime.agentURL, "agent HTTP endpoint")
	cmd.PersistentFlags().String("agent-token", runtime.agentToken, "agent HTTP bearer token")
	cmd.PersistentFlags().Duration("agent-timeout", runtime.agentTimeout, "agent HTTP request timeout")
	cmd.PersistentFlags().Bool("agent-local", runtime.agentLocal, "use an in-process fake agent")

	cmd.AddCommand(newProjectsCommand())
	cmd.AddCommand(newRoomsCommand())
	cmd.AddCommand(newSessionsCommand())
	cmd.AddCommand(newCheckpointsCommand())
	cmd.AddCommand(newArtifactsCommand())
	cmd.AddCommand(newEnvironmentsCommand())
	cmd.AddCommand(newOperationsCommand())
	cmd.AddCommand(newRuntimeObservationsCommand())
	cmd.AddCommand(newAgentsCommand())
	cmd.AddCommand(newLucyCommand())
	return cmd
}

func groupCommand(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, SilenceUsage: true, SilenceErrors: true}
}

func storeCommand(use, short string, handler commandHandler) *cobra.Command {
	return command(use, short, true, false, handler)
}

func agentCommand(use, short string, handler commandHandler) *cobra.Command {
	return command(use, short, false, true, handler)
}

func storeAgentCommand(use, short string, handler commandHandler) *cobra.Command {
	return command(use, short, true, true, handler)
}

func rawCommand(use, short string, handler commandHandler) *cobra.Command {
	return command(use, short, false, false, handler)
}

func command(use, short string, needsStore, needsAgent bool, handler commandHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:                use,
		Short:              short,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime := runtimeFrom(cmd)
			if needsStore {
				if code := runtime.ensureStore(cmd.Context()); code != 0 {
					return exitCodeError(code)
				}
			}
			if needsAgent {
				if code := runtime.ensureAgent(); code != 0 {
					return exitCodeError(code)
				}
			}
			if code := handler(cmd.Context(), runtime, args); code != 0 {
				return exitCodeError(code)
			}
			return nil
		},
	}
	return cmd
}

func runtimeFrom(cmd *cobra.Command) *commandRuntime {
	for current := cmd; current != nil; current = current.Parent() {
		if runtime, ok := current.Context().Value(commandRuntimeKey{}).(*commandRuntime); ok {
			return runtime
		}
	}
	panic("missing command runtime")
}

type commandRuntimeKey struct{}

func (r *commandRuntime) ensureStore(_ context.Context) int {
	if r.store != nil {
		return 0
	}
	store, err := filesystem.New(r.dataDirectory)
	if err != nil {
		fmt.Fprintf(r.stderr, "open data directory: %v\n", err)
		return 1
	}
	r.store = store
	return 0
}

func (r *commandRuntime) ensureAgent() int {
	if r.agentClient != nil {
		return 0
	}
	client, mode, err := buildAgentClient(r.agentURL, r.agentToken, r.agentTimeout, r.agentLocal)
	if err != nil {
		fmt.Fprintf(r.stderr, "configure agent client: %v\n", err)
		return 2
	}
	r.agentClient = client
	r.agentMode = mode
	return 0
}

func (r *commandRuntime) hasAgentURL() bool {
	return strings.TrimSpace(r.agentURL) != ""
}

func init() {
	cobra.EnableCommandSorting = false
}

func (r *commandRuntime) context() context.Context {
	return context.WithValue(context.Background(), commandRuntimeKey{}, r)
}
