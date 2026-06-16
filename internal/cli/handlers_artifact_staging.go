package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/artifact"
	artifactstaging "github.com/stratummc/stratum/internal/artifact/staging"
	artifactstagingsvc "github.com/stratummc/stratum/internal/artifact/stagingservice"
	"github.com/stratummc/stratum/internal/audit"
	"github.com/stratummc/stratum/internal/idgen"
	"github.com/stratummc/stratum/internal/storage/artifactblob"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func artifactStaging(ctx context.Context, store *filesystem.Store, blobRoot string, agentClient agent.AgentClient, agentMode string, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "plan" && args[0] != "list" && args[0] != "inspect" && args[0] != "materialize" && args[0] != "readiness") {
		fmt.Fprintln(stderr, "usage: stratum artifacts staging <plan|list|inspect|materialize|readiness> [flags]")
		return 2
	}
	switch args[0] {
	case "plan":
		return planArtifactStaging(ctx, store, blobRoot, args[1:], stdout, stderr)
	case "list":
		return listArtifactStaging(ctx, store, args[1:], stdout, stderr)
	case "inspect":
		return inspectArtifactStaging(ctx, store, args[1:], stdout, stderr)
	case "materialize":
		return materializeArtifactStaging(ctx, store, blobRoot, agentClient, agentMode, hasAgentURL, args[1:], stdout, stderr)
	case "readiness":
		return artifactStagingReadiness(ctx, store, blobRoot, agentClient, hasAgentURL, args[1:], stdout, stderr)
	default:
		return 2
	}
}

func artifactStagingReadiness(ctx context.Context, store *filesystem.Store, blobRoot string, agentClient agent.AgentClient, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging readiness", stderr)
	sessionID := flags.String("session", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" {
		fmt.Fprintln(stderr, "--session is required")
		return 2
	}
	if !hasAgentURL {
		fmt.Fprintln(stderr, "--agent-url is required for full materialization readiness")
		return 2
	}
	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	result, err := artifactstagingsvc.NewReadinessService(store, blobs, agentClient).Check(ctx, *sessionID)
	if err != nil {
		return reportError(stderr, "check artifact materialization readiness", err)
	}
	fmt.Fprintf(stdout, "session=%s status=%s planned=%d materialized=%d valid=%d missing=%d corrupted=%d unknown=%d issues=%d checkedAt=%s\n", result.SessionID, result.Status, result.PlannedCount, result.MaterializedCount, result.ValidMaterializedCount, result.MissingMaterializedCount, result.CorruptedMaterializedCount, result.UnknownMaterializedCount, len(result.Issues), result.CheckedAt.Format(time.RFC3339Nano))
	for _, issue := range result.Issues {
		fmt.Fprintf(stdout, "issue=%s severity=%s plan=%s artifact=%s message=%q\n", issue.Code, issue.Severity, issue.StagingPlanID, issue.ArtifactID, issue.Message)
	}
	for _, entry := range result.Entries {
		fmt.Fprintf(stdout, "plan=%s artifact=%s artifactStatus=%s materialized=%t verification=%s recommendedAction=%q\n", entry.StagingPlanID, entry.ArtifactID, entry.ArtifactStatus, entry.Materialized, entry.VerificationStatus, entry.RecommendedAction)
	}
	if result.Status != "ready" {
		return 1
	}
	return 0
}

func materializeArtifactStaging(ctx context.Context, store *filesystem.Store, blobRoot string, agentClient agent.AgentClient, agentMode string, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging materialize", stderr)
	planID := flags.String("plan", "", "planned artifact staging plan ID")
	actor := flags.String("actor", "", "actor user ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *planID == "" || strings.TrimSpace(*actor) == "" {
		fmt.Fprintln(stderr, "--plan and --actor are required")
		return 2
	}
	if !hasAgentURL {
		fmt.Fprintln(stderr, "--agent-url is required for artifact materialization")
		return 2
	}
	plan, err := store.GetArtifactStagingPlan(ctx, *planID)
	if err != nil {
		return reportError(stderr, "load artifact staging plan", err)
	}
	if plan.Status != artifactstaging.StatusPlanned {
		return reportError(stderr, "materialize artifact staging", fmt.Errorf("staging plan %q is %q, not planned", plan.ID, plan.Status))
	}
	value, err := store.GetArtifact(ctx, plan.ArtifactID)
	if err != nil {
		return reportError(stderr, "load staged artifact", err)
	}
	if value.Status != artifact.StatusApproved {
		return reportError(stderr, "materialize artifact staging", fmt.Errorf("artifact %q is %q, not approved", value.ID, value.Status))
	}
	if value.SHA256 != plan.ArtifactHash || value.PayloadAlgorithm != artifactblob.Algorithm || value.PayloadStatus != artifact.PayloadAvailable {
		return reportError(stderr, "materialize artifact staging", errors.New("artifact payload metadata does not match planned staging payload"))
	}
	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	metadata, err := blobs.Verify(ctx, value.SHA256)
	if err != nil {
		return reportError(stderr, "verify materialization payload", err)
	}
	if metadata.Algorithm != value.PayloadAlgorithm || metadata.Hash != value.SHA256 || metadata.Reference != value.PayloadReference || metadata.Size != value.SizeBytes {
		return reportError(stderr, "verify materialization payload", errors.New("artifact metadata does not match verified blob"))
	}
	if metadata.Size > agent.MaxArtifactPayloadBytes {
		return reportError(stderr, "materialize artifact staging", fmt.Errorf("artifact payload exceeds %d byte transfer limit", agent.MaxArtifactPayloadBytes))
	}
	path, err := blobs.Path(metadata.Hash)
	if err != nil {
		return reportError(stderr, "resolve materialization payload", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return reportError(stderr, "read materialization payload", err)
	}
	result, err := agentClient.MaterializeArtifact(ctx, agent.ArtifactMaterializationRequest{SessionID: plan.SessionID, ArtifactID: value.ID, StagingPlanID: plan.ID, ArtifactName: value.Name, ArtifactType: string(value.Type), TargetName: plan.TargetStagingName, PayloadAlgorithm: value.PayloadAlgorithm, PayloadHash: value.SHA256, PayloadSize: value.SizeBytes, ActorID: *actor, Payload: payload})
	if err != nil {
		return reportError(stderr, "agent materialize artifact", err)
	}
	auditID, err := idgen.NewID("audit")
	if err != nil {
		return reportError(stderr, "create materialization audit", err)
	}
	event, err := audit.NewEvent(auditID, *actor, "artifact.materialized", "artifact-staging-plan", plan.ID, result.MaterializedAt)
	if err != nil {
		return reportError(stderr, "create materialization audit", err)
	}
	event.ProjectID = plan.ProjectID
	event.Metadata = map[string]string{"sessionId": plan.SessionID, "artifactId": value.ID, "stagingPlanId": plan.ID, "actor": *actor, "payloadHash": value.SHA256, "payloadSize": fmt.Sprintf("%d", value.SizeBytes), "targetName": plan.TargetStagingName, "runtimeRelativePath": result.RuntimeRelativePath, "agentMode": agentMode, "agentId": result.AgentID, "idempotent": fmt.Sprintf("%t", result.Idempotent)}
	if err := store.AppendAuditEvent(ctx, event); err != nil {
		return reportError(stderr, "write materialization audit", err)
	}
	fmt.Fprintf(stdout, "Artifact materialized status=%s session=%s artifact=%s plan=%s target=%s runtimePath=%s hash=%s size=%d agent=%s idempotent=%t. The payload was not installed, mounted, loaded, or executed.\n", result.Status, result.SessionID, result.ArtifactID, result.StagingPlanID, result.TargetName, result.RuntimeRelativePath, result.PayloadHash, result.PayloadSize, result.AgentID, result.Idempotent)
	return 0
}

func planArtifactStaging(ctx context.Context, store *filesystem.Store, blobRoot string, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging plan", stderr)
	sessionID := flags.String("session", "", "session ID")
	artifactID := flags.String("artifact", "", "artifact ID")
	actor := flags.String("actor", "", "actor user ID")
	name := flags.String("name", "", "safe staging name")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" || *artifactID == "" || strings.TrimSpace(*actor) == "" || *name == "" {
		fmt.Fprintln(stderr, "--session, --artifact, --actor, and --name are required")
		return 2
	}
	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	plan, err := artifactstagingsvc.NewWithPayloadVerifier(store, blobs).CreatePlan(ctx, artifactstagingsvc.CreateParams{SessionID: *sessionID, ArtifactID: *artifactID, ActorID: *actor, Name: *name})
	if err != nil {
		return reportError(stderr, "plan artifact staging", err)
	}
	fmt.Fprintf(stdout, "Artifact staging plan %s status=%s session=%s artifact=%s kind=%s target=%s rejection=%q. No payload was copied or mounted.\n", plan.ID, plan.Status, plan.SessionID, plan.ArtifactID, plan.StagingKind, plan.TargetStagingName, plan.RejectionReason)
	return 0
}

func listArtifactStaging(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging list", stderr)
	sessionID := flags.String("session", "", "filter by session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	service := artifactstagingsvc.New(store)
	var values []artifactstaging.Plan
	var err error
	if *sessionID != "" {
		values, err = service.ListBySession(ctx, *sessionID)
	} else {
		values, err = service.List(ctx)
	}
	if err != nil {
		return reportError(stderr, "list artifact staging plans", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", value.ID, value.SessionID, value.ArtifactID, value.StagingKind, value.TargetStagingName, value.Status, value.RejectionReason)
	}
	return 0
}

func inspectArtifactStaging(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging inspect", stderr)
	id := flags.String("id", "", "artifact staging plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	value, err := artifactstagingsvc.New(store).Get(ctx, *id)
	if err != nil {
		return reportError(stderr, "inspect artifact staging plan", err)
	}
	fmt.Fprintf(stdout, "id=%s session=%s project=%s room=%s artifact=%s artifactName=%q artifactType=%s artifactStatus=%s hash=%s kind=%s target=%s actor=%s status=%s rejection=%q createdAt=%s\n", value.ID, value.SessionID, value.ProjectID, value.RoomID, value.ArtifactID, value.ArtifactName, value.ArtifactType, value.ArtifactStatus, value.ArtifactHash, value.StagingKind, value.TargetStagingName, value.ActorID, value.Status, value.RejectionReason, value.CreatedAt.Format(time.RFC3339Nano))
	return 0
}
