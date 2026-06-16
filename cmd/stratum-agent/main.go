package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if _, ok := err.(usageError); ok {
			os.Exit(2)
		}
		os.Exit(1)
	}
	// TODO: add more agent subcommands as runtime supervision grows.
}

type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "stratum-agent",
		Short:         "Run Stratum Agent process supervision endpoints",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newServeCommand())
	return cmd
}

func newServeCommand() *cobra.Command {
	var listen string
	var token string
	var runtimeMode string
	var runtimeRoot string
	var runtimeProfiles string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the Stratum Agent HTTP API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(listen, token, runtimeMode, runtimeRoot, runtimeProfiles)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8787", "listen address")
	cmd.Flags().StringVar(&token, "token", os.Getenv("STRATUM_AGENT_TOKEN"), "optional bearer token")
	cmd.Flags().StringVar(&runtimeMode, "runtime-mode", "dummy-process", "safe runtime mode (dummy-process only)")
	cmd.Flags().StringVar(&runtimeRoot, "runtime-root", ".stratum/runtime", "trusted runtime working root")
	cmd.Flags().StringVar(&runtimeProfiles, "runtime-profiles", "", "trusted local RuntimeProfile JSON configuration")
	return cmd
}

func serve(listen, token, runtimeMode, runtimeRoot, runtimeProfiles string) error {
	if runtimeMode != "dummy-process" {
		return usageError{err: fmt.Errorf("unsupported runtime mode %q; only dummy-process is available", runtimeMode)}
	}

	logger := log.New(os.Stderr, "stratum-agent ", log.LstdFlags)
	registry := runtimeprofile.Builtins()
	if runtimeProfiles != "" {
		profiles, err := runtimeprofile.LoadTrustedFile(runtimeProfiles)
		if err != nil {
			return err
		}
		if err := registry.RegisterAll(profiles); err != nil {
			return fmt.Errorf("register runtime profiles from %q: %w", runtimeProfiles, err)
		}
	}
	runtimeAgent, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, registry, runtimeRoot)
	if err != nil {
		return err
	}
	server := httptransport.NewServer(runtimeAgent, token, logger)
	logger.Printf("listening on %s with %s supervision (auth=%t)", listen, runtimeMode, token != "")
	if err := http.ListenAndServe(listen, server.Handler()); err != nil {
		return err
	}
	return nil
}
