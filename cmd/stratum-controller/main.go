package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	httpapi "github.com/stratummc/stratum/internal/api/http"
	"github.com/stratummc/stratum/internal/controller/agentregistry"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "stratum-controller",
		Short:         "Run Stratum Controller services",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newServeCommand())
	return cmd
}

func newServeCommand() *cobra.Command {
	var listen string
	var dataDir string
	var agentURL string
	var agentToken string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Stratum Controller HTTP API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(listen, dataDir, agentURL, agentToken)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "0.0.0.0:8080", "listen address")
	cmd.Flags().StringVar(&dataDir, "data-dir", "data", "persistent data directory")
	cmd.Flags().StringVar(&agentURL, "agent-url", "", "default agent HTTP endpoint")
	cmd.Flags().StringVar(&agentToken, "agent-token", os.Getenv("STRATUM_AGENT_TOKEN"), "agent bearer token")
	return cmd
}

func serve(listen, dataDir, agentURL, agentToken string) error {
	logger := log.New(os.Stderr, "stratum-controller ", log.LstdFlags)

	store, err := filesystem.New(dataDir)
	if err != nil {
		return fmt.Errorf("open data directory %q: %w", dataDir, err)
	}

	agentStore := filesystem.NewAgentRegistryStore(dataDir)
	registry := agentregistry.New(agentStore, 2*time.Minute)
	logger.Printf("agent registry initialized (stale timeout=2m)")

	var agentClient agent.AgentClient
	if agentURL != "" {
		timeout := 120 * time.Second
		client, err := httptransport.NewClient(agentURL, agentToken, timeout)
		if err != nil {
			return fmt.Errorf("create agent client: %w", err)
		}
		agentClient = client
		logger.Printf("default agent client configured: %s", agentURL)
	}

	server := httpapi.NewServerWithServices(store, agentClient)
	server.WithAgentRegistry(registry)

	logger.Printf("listening on %s (data-dir=%s)", listen, dataDir)
	return http.ListenAndServe(listen, server.Handler())
}
