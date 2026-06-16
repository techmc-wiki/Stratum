package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/stratummc/stratum/internal/agent"
)

func sessionsMCDRConfigStub(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "inspect" {
		return sessionsMCDRConfigStubInspect(ctx, agentClient, args[1:], stdout, stderr)
	}
	fmt.Fprintln(stderr, "sessions mcdr-config-stub: unknown subcommand (try 'inspect')")
	return 2
}

func sessionsMCDRConfigStubInspect(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions mcdr-config-stub inspect", stderr)
	sessionID := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" {
		fmt.Fprintln(stderr, "sessions mcdr-config-stub inspect: --id is required")
		return 2
	}
	result, err := agentClient.InspectMCDRConfigStub(ctx, *sessionID)
	if err != nil {
		return reportError(stderr, "inspect MCDR config stub", err)
	}
	fmt.Fprintf(stdout, "MCDR Config Stub Inspection (session %s):\n", result.SessionID)
	fmt.Fprintf(stdout, "  Exists:    %t\n", result.Exists)
	fmt.Fprintf(stdout, "  Valid:     %t\n", result.Valid)
	fmt.Fprintf(stdout, "  Status:    %s\n", result.Status)
	fmt.Fprintf(stdout, "  Path:      %s\n", result.Path)
	if result.PlannedConfigYMLPath != "" {
		fmt.Fprintf(stdout, "  Config:    %s\n", result.PlannedConfigYMLPath)
	}
	if result.PlannedServerPropertiesPath != "" {
		fmt.Fprintf(stdout, "  Server:    %s\n", result.PlannedServerPropertiesPath)
	}
	if result.PlannedEULAPath != "" {
		fmt.Fprintf(stdout, "  EULA:      %s\n", result.PlannedEULAPath)
	}
	if len(result.Issues) > 0 {
		fmt.Fprintln(stdout, "  Issues:")
		for _, issue := range result.Issues {
			fmt.Fprintf(stdout, "    - %s\n", issue)
		}
	}
	fmt.Fprintf(stdout, "  Checked:   %s\n", result.CheckedAt.Format(time.RFC3339))
	if !result.Exists || !result.Valid {
		return 1
	}
	return 0
}
