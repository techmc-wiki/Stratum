package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
)

func agentRuntimeProfiles(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("agents runtime-profiles", stderr)
	id := flags.String("id", local.DefaultAgentID, "agent ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id != local.DefaultAgentID {
		fmt.Fprintf(stderr, "agent %q not found\n", *id)
		return 1
	}
	profiles, err := agentClient.RuntimeProfiles(ctx)
	if err != nil {
		return reportError(stderr, "list runtime profiles", err)
	}
	for _, value := range profiles {
		fmt.Fprintf(stdout, "%s\t%s\t%s\tenabled=%t\tstop=%s\t%s\n", value.ID, value.Name, value.RuntimeType, value.Enabled, value.StopStrategy, value.Notes)
	}
	return 0
}

func listAgents(ctx context.Context, agentClient agent.AgentClient, stdout, stderr io.Writer) int {
	info, err := agentClient.Info(ctx)
	if err != nil {
		return reportError(stderr, "list agents", err)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\n", info.ID, info.Status, info.RuntimeEndpoint)
	return 0
}

func inspectAgent(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("agents inspect", stderr)
	id := flags.String("id", local.DefaultAgentID, "agent ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id != local.DefaultAgentID {
		fmt.Fprintf(stderr, "agent %q not found\n", *id)
		return 1
	}
	info, err := agentClient.Info(ctx)
	if err != nil {
		return reportError(stderr, "inspect agent", err)
	}
	report, err := agentClient.ReportResources(ctx)
	if err != nil {
		return reportError(stderr, "report agent resources", err)
	}
	fmt.Fprintf(stdout, "id=%s status=%s endpoint=%s capabilities=%s cpu=%d memory=%d/%dMB disk=%d/%dMB running=%d\n",
		info.ID, info.Status, info.RuntimeEndpoint, strings.Join(info.Capabilities, ","), report.CPUCapacity,
		report.MemoryUsedMB, report.MemoryTotalMB, report.DiskUsedMB, report.DiskTotalMB, report.RunningSessions)
	return 0
}

func agentResources(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("agents resources", stderr)
	id := flags.String("id", local.DefaultAgentID, "agent ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id != local.DefaultAgentID {
		fmt.Fprintf(stderr, "agent %q not found\n", *id)
		return 1
	}
	report, err := agentClient.ReportResources(ctx)
	if err != nil {
		return reportError(stderr, "report agent resources", err)
	}
	fmt.Fprintf(stdout, "agent=%s cpu=%d memory=%d/%dMB disk=%d/%dMB running=%d reported=%s\n",
		report.AgentID, report.CPUCapacity, report.MemoryUsedMB, report.MemoryTotalMB,
		report.DiskUsedMB, report.DiskTotalMB, report.RunningSessions, report.ReportedAt.Format(time.RFC3339))
	return 0
}
