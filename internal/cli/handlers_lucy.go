package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func lucyPlan(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	environmentID, ok := singleArg(args, "environment-id", stderr)
	if !ok {
		return 2
	}
	env, err := store.GetEnvironment(ctx, environmentID)
	if err != nil {
		return reportError(stderr, "get environment", err)
	}
	request := materializationRequestForEnvironment(env, "lucy-plan-"+env.ID, "cli")
	result, err := agentClient.MaterializeEnvironment(ctx, request)
	if err != nil {
		return reportError(stderr, "lucy plan", err)
	}
	fmt.Fprintf(stdout, "lucy plan environment=%s session=%s status=%s resolution=%s\n", env.ID, result.SessionID, result.Status, result.LucyResolutionStatus)
	fmt.Fprintf(stdout, "actions=%s warnings=%s errors=%s requiresLockUpdate=%s\n", metadataValue(result.Metadata, "lucyPlanActionCount", "0"), metadataValue(result.Metadata, "lucyPlanWarningCount", "0"), metadataValue(result.Metadata, "lucyPlanErrorCount", "0"), metadataValue(result.Metadata, "lucyPlanRequiresLockUpdate", "false"))
	printLucyError(stdout, result.Metadata)
	return 0
}

func lucyLock(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	result, ok := materializeLucySession(ctx, store, agentClient, args, "lucy lock", stderr)
	if !ok {
		return 1
	}
	fmt.Fprintf(stdout, "lucy lock session=%s status=%s resolution=%s\n", result.SessionID, result.Status, result.LucyResolutionStatus)
	fmt.Fprintf(stdout, "lockHash=%s lockPath=%s packages=%s artifacts=%s\n", result.LucyLockHash, result.LucyLockPath, metadataValue(result.Metadata, "lucyLockPackageCount", "0"), metadataValue(result.Metadata, "lucyLockArtifactCount", "0"))
	printLucyError(stdout, result.Metadata)
	return 0
}

func lucyStatus(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	sessionID, ok := singleArg(args, "session-id", stderr)
	if !ok {
		return 2
	}
	status, err := agentClient.GetSessionRuntimeStatus(ctx, sessionID)
	if err != nil {
		return reportError(stderr, "lucy status", err)
	}
	fmt.Fprintf(stdout, "lucy status session=%s\n", sessionID)
	if status.EnvironmentManifest == nil || !status.EnvironmentManifest.Exists {
		fmt.Fprintln(stdout, "environmentManifest=missing")
		fmt.Fprintln(stdout, "missing=environment-manifest")
		fmt.Fprintln(stdout, "drifted=")
		return 0
	}
	manifest := status.EnvironmentManifest
	fmt.Fprintf(stdout, "environmentManifest=%s status=%s lockHash=%s\n", manifest.RuntimeRelativePath, manifest.Status, manifest.LucyLockHash)
	fmt.Fprintln(stdout, "missing=")
	fmt.Fprintln(stdout, "drifted=")
	return 0
}

func lucyVerify(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	result, ok := materializeLucySession(ctx, store, agentClient, args, "lucy verify", stderr)
	if !ok {
		return 1
	}
	fmt.Fprintf(stdout, "lucy verify session=%s status=%s ok=%t\n", result.SessionID, result.LucyIntegrityStatus, result.LucyIntegrityStatus == "ok" || result.LucyIntegrityStatus == "not_checked")
	fmt.Fprintf(stdout, "missing=%s corrupt=%s checked=%s\n", metadataValue(result.Metadata, "lucyIntegrityMissing", "0"), metadataValue(result.Metadata, "lucyIntegrityCorrupt", "0"), metadataValue(result.Metadata, "lucyIntegrityChecked", "0"))
	printLucyError(stdout, result.Metadata)
	return 0
}

func lucyInstall(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	result, ok := materializeLucySession(ctx, store, agentClient, args, "lucy install", stderr)
	if !ok {
		return 1
	}
	status := result.LucyInstallStatus
	if status == "" {
		status = "not_capable"
	}
	fmt.Fprintf(stdout, "lucy install session=%s status=%s installed=%d failed=%d totalSize=%d\n", result.SessionID, status, result.LucyInstalledCount, result.LucyFailedCount, result.LucyInstallTotalSize)
	printLucyError(stdout, result.Metadata)
	return 0
}

func materializeLucySession(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, label string, stderr io.Writer) (agent.EnvironmentMaterializationResult, bool) {
	sessionID, ok := singleArg(args, "session-id", stderr)
	if !ok {
		return agent.EnvironmentMaterializationResult{}, false
	}
	sess, err := store.GetSession(ctx, sessionID)
	if err != nil {
		return agent.EnvironmentMaterializationResult{}, reportError(stderr, "get session", err) == 0
	}
	if strings.TrimSpace(sess.EnvironmentID) == "" {
		fmt.Fprintln(stderr, "session has no environment")
		return agent.EnvironmentMaterializationResult{}, false
	}
	env, err := store.GetEnvironment(ctx, sess.EnvironmentID)
	if err != nil {
		return agent.EnvironmentMaterializationResult{}, reportError(stderr, "get environment", err) == 0
	}
	request := materializationRequestForEnvironment(env, sess.ID, "cli")
	result, err := agentClient.MaterializeEnvironment(ctx, request)
	if err != nil {
		return agent.EnvironmentMaterializationResult{}, reportError(stderr, label, err) == 0
	}
	return result, true
}

func materializationRequestForEnvironment(env environment.Environment, sessionID, actorID string) agent.EnvironmentMaterializationRequest {
	return agent.EnvironmentMaterializationRequest{
		SessionID:              sessionID,
		EnvironmentID:          env.ID,
		EnvironmentName:        env.Name,
		MinecraftVersion:       env.MinecraftVersion,
		JavaVersion:            env.JavaVersion,
		LoaderType:             string(env.LoaderType),
		LoaderVersion:          env.LoaderVersion,
		ServerCore:             string(env.ServerCore),
		MCDRRequired:           env.MCDRRequired,
		CarpetRequired:         env.CarpetRequired,
		LucyManifestRef:        env.LucyManifestRef,
		LucyLockRef:            env.LucyLockRef,
		RuntimeProfileID:       env.RuntimeProfileID,
		RuntimeProfileRequired: env.RuntimeProfileRequired,
		ActorID:                actorID,
	}
}

func singleArg(args []string, name string, stderr io.Writer) (string, bool) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintf(stderr, "%s is required\n", name)
		return "", false
	}
	return args[0], true
}

func metadataValue(metadata map[string]string, key, fallback string) string {
	if metadata == nil || metadata[key] == "" {
		return fallback
	}
	return metadata[key]
}

func printLucyError(stdout io.Writer, metadata map[string]string) {
	if metadata == nil || metadata["lucyResolutionError"] == "" {
		return
	}
	fmt.Fprintf(stdout, "error=%s code=%s\n", metadata["lucyResolutionError"], metadata["lucyResolutionErrorCode"])
}
