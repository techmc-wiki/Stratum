package process

import (
	"context"
	"fmt"

	"github.com/stratummc/stratum/internal/agent"
)

func (s *Supervisor) SessionReadyForStart(ctx context.Context, sessionID string) (agent.SessionStartReadiness, error) {
	if _, err := NewSessionRuntimeLayout(s.runtimeRoot, sessionID); err != nil {
		return agent.SessionStartReadiness{}, err
	}
	status, err := s.GetSessionRuntimeStatus(ctx, sessionID)
	if err != nil {
		return agent.SessionStartReadiness{}, err
	}
	result := agent.SessionStartReadiness{SessionID: sessionID, CheckedAt: s.now(), Status: "not_ready", Issues: []agent.SessionStartReadinessIssue{}}
	result.RuntimeStatusSummary = agent.SessionStartReadinessSummary{
		RuntimeRootExists: status.RuntimeRootExists, SessionRootExists: status.SessionRootExists,
		WorkDirExists: status.WorkDirExists, ConfigDirExists: status.ConfigDirExists, LogsDirExists: status.LogsDirExists,
	}
	add := func(code, message, severity string) {
		result.Issues = append(result.Issues, agent.SessionStartReadinessIssue{Code: code, Message: message, Severity: severity})
	}
	if !status.RuntimeRootExists {
		add("runtime_root_missing", "runtime root does not exist", "error")
	}
	if !status.SessionRootExists {
		add("session_root_missing", "session runtime root does not exist", "error")
	}
	if !status.ConfigDirExists {
		add("config_dir_missing", "session config directory does not exist", "error")
	}
	if !status.WorkDirExists {
		add("work_dir_missing", "session work directory does not exist", "error")
	}
	if !status.LogsDirExists {
		add("logs_dir_missing", "session logs directory does not exist", "error")
	}
	if status.EnvironmentManifest == nil || !status.EnvironmentManifest.Exists {
		add("environment_manifest_missing", "environment materialization manifest does not exist", "error")
	} else {
		result.RuntimeStatusSummary.EnvironmentManifestExists = true
		result.RuntimeStatusSummary.EnvironmentManifestStatus = status.EnvironmentManifest.Status
		if status.EnvironmentManifest.ErrorMessage != "" {
			add("environment_manifest_malformed", "environment materialization manifest is malformed", "error")
		} else if status.EnvironmentManifest.Status != "prepared" {
			add("environment_not_prepared", fmt.Sprintf("environment materialization status is %q", status.EnvironmentManifest.Status), "error")
		}
		if status.EnvironmentManifest.MCDRRequired && (status.MCDRLayout == nil || !status.MCDRLayout.MCDRRootExists) {
			add("mcdr_layout_missing", "environment requires MCDR but the MCDR runtime layout is missing", "error")
		}
	}
	if status.ProcessStatus != nil {
		result.RuntimeStatusSummary.ProcessState = status.ProcessStatus.Status
		if status.ProcessStatus.Crashed || status.ProcessStatus.Status == string(StatusCrashed) {
			add("process_crashed", "session runtime process is crashed", "error")
		} else if status.ProcessStatus.Status == string(StatusRunning) || status.ProcessStatus.Status == string(StatusStarting) || status.ProcessStatus.Status == string(StatusStopping) {
			add("process_active", fmt.Sprintf("session runtime process is %s", status.ProcessStatus.Status), "error")
		}
	} else {
		result.RuntimeStatusSummary.ProcessState = string(StatusNotStarted)
	}
	if status.AppliedArtifacts != nil && status.AppliedArtifacts.ManifestExists {
		verification, verifyErr := VerifyAllAppliedArtifacts(ctx, s.runtimeRoot, sessionID, s.now())
		if verifyErr != nil {
			result.RuntimeStatusSummary.AppliedArtifactsError = 1
			add("applied_artifacts_verify_error", verifyErr.Error(), "error")
		} else {
			result.RuntimeStatusSummary.AppliedArtifactsTotal = verification.Total
			result.RuntimeStatusSummary.AppliedArtifactsValid = verification.ValidCount
			result.RuntimeStatusSummary.AppliedArtifactsMissing = verification.MissingCount
			result.RuntimeStatusSummary.AppliedArtifactsCorrupted = verification.CorruptedCount
			result.RuntimeStatusSummary.AppliedArtifactsError = verification.ErrorCount
			if verification.MissingCount > 0 {
				add("applied_artifact_missing", "one or more applied artifacts are missing", "error")
			}
			if verification.CorruptedCount > 0 {
				add("applied_artifact_corrupted", "one or more applied artifacts are corrupted", "error")
			}
			if verification.ErrorCount > 0 {
				add("applied_artifact_error", "one or more applied artifacts could not be verified", "error")
			}
		}
	}
	result.Ready = len(result.Issues) == 0
	if result.Ready {
		result.Status = "ready"
	}
	return result, nil
}
