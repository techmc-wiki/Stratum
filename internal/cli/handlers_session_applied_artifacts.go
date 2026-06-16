package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/stratummc/stratum/internal/agent"
)

func sessionsAppliedArtifacts(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "inspect" {
		return sessionsAppliedArtifactsInspect(ctx, agentClient, args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "verify" {
		return sessionsAppliedArtifactsVerify(ctx, agentClient, args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "verify-all" {
		return sessionsAppliedArtifactsVerifyAll(ctx, agentClient, args[1:], stdout, stderr)
	}
	flags := newFlagSet("sessions applied-artifacts", stderr)
	sessionID := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" {
		fmt.Fprintln(stderr, "session applied-artifacts: --id is required")
		return 2
	}
	result, err := agentClient.ListAppliedArtifacts(ctx, *sessionID)
	if err != nil {
		return reportError(stderr, "list applied artifacts", err)
	}
	if len(result.Records) == 0 {
		fmt.Fprintln(stdout, "No applied artifacts.")
		return 0
	}
	fmt.Fprintf(stdout, "Applied Artifacts (session %s):\n", result.SessionID)
	for _, r := range result.Records {
		fmt.Fprintf(stdout, "  %s: %s -> %s (status=%s, size=%d, action=%s, applied=%s)\n", r.ApplyPlanID, r.SourceRuntimeRelativePath, r.TargetRuntimeRelativePath, r.Status, r.PayloadSize, r.Action, r.AppliedAt.Format(time.RFC3339))
	}
	return 0
}

func sessionsAppliedArtifactsInspect(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions applied-artifacts inspect", stderr)
	sessionID := flags.String("id", "", "session ID")
	applyPlanID := flags.String("plan", "", "apply plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" || *applyPlanID == "" {
		fmt.Fprintln(stderr, "sessions applied-artifacts inspect: --id and --plan are required")
		return 2
	}
	record, err := agentClient.InspectAppliedArtifact(ctx, *sessionID, *applyPlanID)
	if err != nil {
		return reportError(stderr, "inspect applied artifact", err)
	}
	fmt.Fprintf(stdout, "Apply Plan ID:              %s\n", record.ApplyPlanID)
	fmt.Fprintf(stdout, "Session ID:                 %s\n", record.SessionID)
	fmt.Fprintf(stdout, "Artifact ID:                %s\n", record.ArtifactID)
	fmt.Fprintf(stdout, "Staging Plan ID:            %s\n", record.StagingPlanID)
	fmt.Fprintf(stdout, "Source Runtime Path:        %s\n", record.SourceRuntimeRelativePath)
	fmt.Fprintf(stdout, "Target Runtime Path:        %s\n", record.TargetRuntimeRelativePath)
	fmt.Fprintf(stdout, "Target Root:                %s\n", record.TargetRoot)
	fmt.Fprintf(stdout, "Target Relative Path:       %s\n", record.TargetRelativePath)
	fmt.Fprintf(stdout, "Payload Algorithm:          %s\n", record.PayloadAlgorithm)
	fmt.Fprintf(stdout, "Payload Hash:               %s\n", record.PayloadHash)
	fmt.Fprintf(stdout, "Payload Size:               %d\n", record.PayloadSize)
	fmt.Fprintf(stdout, "Action:                     %s\n", record.Action)
	fmt.Fprintf(stdout, "Status:                     %s\n", record.Status)
	if record.ActorID != "" {
		fmt.Fprintf(stdout, "Actor ID:                   %s\n", record.ActorID)
	}
	fmt.Fprintf(stdout, "Applied At:                 %s\n", record.AppliedAt.Format(time.RFC3339))
	return 0
}

func sessionsAppliedArtifactsVerify(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions applied-artifacts verify", stderr)
	sessionID := flags.String("id", "", "session ID")
	applyPlanID := flags.String("plan", "", "apply plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" || *applyPlanID == "" {
		fmt.Fprintln(stderr, "sessions applied-artifacts verify: --id and --plan are required")
		return 2
	}
	result, err := agentClient.VerifyAppliedArtifact(ctx, *sessionID, *applyPlanID)
	if err != nil {
		return reportError(stderr, "verify applied artifact", err)
	}
	fmt.Fprintf(stdout, "Session ID:                 %s\n", result.SessionID)
	fmt.Fprintf(stdout, "Apply Plan ID:              %s\n", result.ApplyPlanID)
	fmt.Fprintf(stdout, "Artifact ID:                %s\n", result.ArtifactID)
	fmt.Fprintf(stdout, "Target Runtime Path:        %s\n", result.TargetRuntimeRelativePath)
	fmt.Fprintf(stdout, "Expected Hash:              %s\n", result.ExpectedHash)
	if result.ActualHash != "" {
		fmt.Fprintf(stdout, "Actual Hash:                %s\n", result.ActualHash)
	}
	fmt.Fprintf(stdout, "Payload Size:               %d\n", result.PayloadSize)
	fmt.Fprintf(stdout, "Actual Size:                %d\n", result.ActualSize)
	fmt.Fprintf(stdout, "Status:                     %s\n", result.Status)
	fmt.Fprintf(stdout, "Verified At:                %s\n", result.VerifiedAt.Format(time.RFC3339))
	if result.ErrorMessage != "" {
		fmt.Fprintf(stdout, "Error:                      %s\n", result.ErrorMessage)
	}
	if result.Status != "valid" {
		return 1
	}
	return 0
}

func sessionsAppliedArtifactsVerifyAll(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions applied-artifacts verify-all", stderr)
	sessionID := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" {
		fmt.Fprintln(stderr, "sessions applied-artifacts verify-all: --id is required")
		return 2
	}
	result, err := agentClient.VerifyAllAppliedArtifacts(ctx, *sessionID)
	if err != nil {
		return reportError(stderr, "verify all applied artifacts", err)
	}
	fmt.Fprintf(stdout, "Session ID:      %s\n", result.SessionID)
	fmt.Fprintf(stdout, "Verified At:     %s\n", result.VerifiedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Total:           %d\n", result.Total)
	fmt.Fprintf(stdout, "Valid:           %d\n", result.ValidCount)
	fmt.Fprintf(stdout, "Missing:         %d\n", result.MissingCount)
	fmt.Fprintf(stdout, "Corrupted:       %d\n", result.CorruptedCount)
	fmt.Fprintf(stdout, "Error:           %d\n", result.ErrorCount)
	fmt.Fprintln(stdout)
	for _, entry := range result.Entries {
		fmt.Fprintf(stdout, "Apply Plan ID:   %s\n", entry.ApplyPlanID)
		fmt.Fprintf(stdout, "  Artifact ID:   %s\n", entry.ArtifactID)
		fmt.Fprintf(stdout, "  Target Path:   %s\n", entry.TargetRuntimeRelativePath)
		fmt.Fprintf(stdout, "  Status:        %s\n", entry.Status)
		if entry.ErrorMessage != "" {
			fmt.Fprintf(stdout, "  Error:         %s\n", entry.ErrorMessage)
		}
		fmt.Fprintln(stdout)
	}
	if result.ValidCount != result.Total {
		return 1
	}
	return 0
}
