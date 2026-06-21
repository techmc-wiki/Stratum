package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	artifactapply "github.com/stratummc/stratum/internal/artifact/apply"
	artifactapplysvc "github.com/stratummc/stratum/internal/artifact/applyservice"
	"github.com/stratummc/stratum/internal/session"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func artifactApply(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: stratum artifacts apply <plan|list|inspect|dry-run|execute> [flags]")
		return 2
	}
	switch args[0] {
	case "plan":
		return artifactApplyPlan(ctx, store, agentClient, args[1:], stdout, stderr)
	case "list":
		return artifactApplyList(ctx, store, args[1:], stdout, stderr)
	case "inspect":
		return artifactApplyInspect(ctx, store, args[1:], stdout, stderr)
	case "dry-run":
		return artifactApplyDryRun(ctx, store, agentClient, args[1:], stdout, stderr)
	case "execute":
		return artifactApplyExecute(ctx, store, agentClient, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown artifacts apply command %q\n", args[0])
		return 2
	}
}

func artifactApplyPlan(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply plan", stderr)
	sessionID := flags.String("session", "", "session ID")
	stagingPlanID := flags.String("staging-plan", "", "staging plan ID")
	actor := flags.String("actor", "", "actor ID")
	target := flags.String("target", "", "target relative path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" || *stagingPlanID == "" || *actor == "" || *target == "" {
		fmt.Fprintln(stderr, "--session, --staging-plan, --actor, and --target are required")
		return 2
	}
	service := artifactapplysvc.New(store, agentClient)
	plan, err := service.CreatePlan(ctx, artifactapplysvc.CreateParams{SessionID: *sessionID, StagingPlanID: *stagingPlanID, ActorID: *actor, TargetPath: *target})
	if err != nil {
		return reportError(stderr, "create artifact apply plan", err)
	}
	fmt.Fprintf(stdout, "Artifact apply plan %s status=%s kind=%s root=%s target=%s rejection=%q. No file was copied, mounted, installed, or executed.\n", plan.ID, plan.Status, plan.ApplyKind, plan.TargetRoot, plan.TargetRelativePath, plan.RejectionReason)
	return 0
}

func artifactApplyList(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply list", stderr)
	sessionID := flags.String("session", "", "filter by session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	service := artifactapplysvc.New(store, nil)
	var values []artifactapply.Plan
	var err error
	if *sessionID != "" {
		values, err = service.ListBySession(ctx, *sessionID)
	} else {
		values, err = service.List(ctx)
	}
	if err != nil {
		return reportError(stderr, "list artifact apply plans", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", value.ID, value.SessionID, value.ArtifactID, value.Status, value.ApplyKind, value.TargetRelativePath)
	}
	return 0
}

func artifactApplyInspect(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply inspect", stderr)
	id := flags.String("id", "", "apply plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	service := artifactapplysvc.New(store, nil)
	plan, err := service.Get(ctx, *id)
	if err != nil {
		return reportError(stderr, "get artifact apply plan", err)
	}
	fmt.Fprintf(stdout, "Apply Plan %s\n", plan.ID)
	fmt.Fprintf(stdout, "Session: %s\n", plan.SessionID)
	fmt.Fprintf(stdout, "Artifact: %s\n", plan.ArtifactID)
	fmt.Fprintf(stdout, "Status: %s\n", plan.Status)
	fmt.Fprintf(stdout, "ApplyKind: %s\n", plan.ApplyKind)
	fmt.Fprintf(stdout, "TargetRoot: %s\n", plan.TargetRoot)
	fmt.Fprintf(stdout, "TargetPath: %s\n", plan.TargetRelativePath)
	fmt.Fprintf(stdout, "Hash: %s\n", plan.MaterializedArtifactHash)
	fmt.Fprintf(stdout, "Name: %s\n", plan.MaterializedArtifactName)
	if plan.RejectionReason != "" {
		fmt.Fprintf(stdout, "Rejection: %s\n", plan.RejectionReason)
	}
	return 0
}

func artifactApplyDryRun(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply dry-run", stderr)
	planID := flags.String("plan", "", "apply plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *planID == "" {
		fmt.Fprintln(stderr, "--plan is required")
		return 2
	}
	service := artifactapplysvc.New(store, nil)
	plan, err := service.Get(ctx, *planID)
	if err != nil {
		return reportError(stderr, "get artifact apply plan", err)
	}
	if plan.Status != artifactapply.StatusPlanned {
		fmt.Fprintf(stderr, "apply plan status is %s, not planned\n", plan.Status)
		return 1
	}
	req := agent.ArtifactApplyDryRunRequest{ApplyPlanID: plan.ID, SessionID: plan.SessionID, StagingPlanID: plan.SourceStagingPlanID, ArtifactID: plan.ArtifactID, TargetRoot: string(plan.TargetRoot), TargetRelativePath: plan.TargetRelativePath, ExpectedHash: plan.MaterializedArtifactHash}
	result, err := agentClient.DryRunArtifactApply(ctx, req)
	if err != nil {
		return reportError(stderr, "dry-run artifact apply", err)
	}
	fmt.Fprintf(stdout, "Dry-run result for apply plan %s:\n", result.ApplyPlanID)
	fmt.Fprintf(stdout, "Status: %s\n", result.Status)
	fmt.Fprintf(stdout, "Action: %s\n", result.Action)
	fmt.Fprintf(stdout, "Source: %s\n", result.SourceRuntimeRelativePath)
	fmt.Fprintf(stdout, "Target: %s\n", result.PlannedTargetRuntimeRelativePath)
	if len(result.Issues) > 0 {
		fmt.Fprintf(stdout, "Issues:\n")
		for _, issue := range result.Issues {
			fmt.Fprintf(stdout, "  - %s\n", issue)
		}
	}
	fmt.Fprintln(stdout, "No files were copied, mounted, installed, or executed.")
	return 0
}

func artifactApplyExecute(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply execute", stderr)
	planID := flags.String("plan", "", "apply plan ID")
	preOpCheckpoint := flags.Bool("pre-op-checkpoint", false, "create world snapshot before execution")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *planID == "" {
		fmt.Fprintln(stderr, "--plan is required")
		return 2
	}
	service := artifactapplysvc.New(store, nil)
	plan, err := service.Get(ctx, *planID)
	if err != nil {
		return reportError(stderr, "get artifact apply plan", err)
	}
	if plan.Status != artifactapply.StatusPlanned {
		fmt.Fprintf(stderr, "apply plan status is %s, not planned\n", plan.Status)
		return 1
	}

	if *preOpCheckpoint {
		if cpErr := createPreOpCheckpoint(ctx, store, agentClient, plan.SessionID, plan.ActorID, "Pre-operation checkpoint before artifact apply"); cpErr != nil {
			fmt.Fprintf(stderr, "pre-op checkpoint failed: %v (continuing with apply)\n", cpErr)
		} else {
			fmt.Fprintf(stdout, "Pre-operation checkpoint created for session %s.\n", plan.SessionID)
		}
	}

	req := agent.ArtifactApplyExecuteRequest{ApplyPlanID: plan.ID, SessionID: plan.SessionID, StagingPlanID: plan.SourceStagingPlanID, ArtifactID: plan.ArtifactID, TargetRoot: string(plan.TargetRoot), TargetRelativePath: plan.TargetRelativePath, ExpectedHash: plan.MaterializedArtifactHash}
	result, err := agentClient.ExecuteArtifactApply(ctx, req)
	if err != nil {
		return reportError(stderr, "execute artifact apply", err)
	}
	fmt.Fprintf(stdout, "Apply execution result for plan %s:\n", result.ApplyPlanID)
	fmt.Fprintf(stdout, "Status: %s\n", result.Status)
	fmt.Fprintf(stdout, "Action: %s\n", result.Action)
	fmt.Fprintf(stdout, "Source: %s\n", result.SourcePath)
	fmt.Fprintf(stdout, "Target: %s\n", result.TargetPath)
	fmt.Fprintf(stdout, "Copied: %d bytes\n", result.CopiedBytes)
	fmt.Fprintf(stdout, "Hash: %s\n", result.VerifiedTargetHash)
	if len(result.Issues) > 0 {
		fmt.Fprintf(stdout, "Issues:\n")
		for _, issue := range result.Issues {
			fmt.Fprintf(stdout, "  - %s\n", issue)
		}
	}
	return 0
}

func buildAgentClient(rawURL, token string, timeout time.Duration, useLocal bool) (agent.AgentClient, string, error) {
	if strings.TrimSpace(rawURL) == "" {
		if useLocal {
			return local.NewFake(), "local", nil
		}
		return nil, "", fmt.Errorf("requires --agent-url unless --agent-local is set")
	}
	client, err := httptransport.NewClient(rawURL, token, timeout)
	if err != nil {
		return nil, "", err
	}
	return client, "http", nil
}

func parseSessionType(value string) (session.Type, bool) {
	candidate := session.Type(value)
	switch candidate {
	case session.TypeShared, session.TypeFork, session.TypePrivate, session.TypeReview, session.TypeArchived:
		return candidate, true
	default:
		return "", false
	}
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func reportError(stderr io.Writer, action string, err error) int {
	fmt.Fprintf(stderr, "%s: %v\n", action, err)
	return 1
}
