package stagingservice

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/artifact"
	artifactstaging "github.com/stratummc/stratum/internal/artifact/staging"
	"github.com/stratummc/stratum/internal/session"
)

type ReadinessRepository interface {
	GetSession(context.Context, string) (session.Session, error)
	GetArtifact(context.Context, string) (artifact.Artifact, error)
	ListArtifactStagingPlansBySession(context.Context, string) ([]artifactstaging.Plan, error)
}

type MaterializationVerifier interface {
	VerifyMaterializedArtifacts(context.Context, string) (agent.MaterializedArtifactsVerification, error)
}

type ReadinessIssue struct {
	Code          string
	Message       string
	StagingPlanID string
	ArtifactID    string
	Severity      string
}

type ReadinessEntry struct {
	StagingPlanID      string
	ArtifactID         string
	ArtifactStatus     string
	Materialized       bool
	VerificationStatus string
	RecommendedAction  string
}

type ReadinessResult struct {
	SessionID                  string
	CheckedAt                  time.Time
	Status                     string
	PlannedCount               int
	MaterializedCount          int
	ValidMaterializedCount     int
	MissingMaterializedCount   int
	CorruptedMaterializedCount int
	UnknownMaterializedCount   int
	Issues                     []ReadinessIssue
	Entries                    []ReadinessEntry
}

type ReadinessService struct {
	repository              ReadinessRepository
	payloadVerifier         PayloadVerifier
	materializationVerifier MaterializationVerifier
	now                     func() time.Time
}

func NewReadinessService(repository ReadinessRepository, payloadVerifier PayloadVerifier, materializationVerifier MaterializationVerifier) *ReadinessService {
	return &ReadinessService{repository: repository, payloadVerifier: payloadVerifier, materializationVerifier: materializationVerifier, now: func() time.Time { return time.Now().UTC() }}
}

func (s *ReadinessService) Check(ctx context.Context, sessionID string) (ReadinessResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return ReadinessResult{}, fmt.Errorf("session is required")
	}
	if _, err := s.repository.GetSession(ctx, sessionID); err != nil {
		return ReadinessResult{}, fmt.Errorf("load session: %w", err)
	}
	plans, err := s.repository.ListArtifactStagingPlansBySession(ctx, sessionID)
	if err != nil {
		return ReadinessResult{}, fmt.Errorf("list artifact staging plans: %w", err)
	}
	result := ReadinessResult{SessionID: sessionID, CheckedAt: s.now(), Status: "ready", Issues: []ReadinessIssue{}, Entries: []ReadinessEntry{}}
	planned := make(map[string]artifactstaging.Plan)
	controllerReady := make(map[string]bool)
	for _, plan := range plans {
		if plan.Status == artifactstaging.StatusPlanned {
			planned[plan.ID] = plan
		}
	}
	result.PlannedCount = len(planned)
	if result.PlannedCount == 0 {
		result.Status = "not_ready"
		result.Issues = append(result.Issues, readinessIssue("no_planned_artifacts", "session has no planned artifact staging entries", "", ""))
	}

	entries := make(map[string]ReadinessEntry, len(planned))
	for id, plan := range planned {
		entry := ReadinessEntry{StagingPlanID: id, ArtifactID: plan.ArtifactID, VerificationStatus: "not_materialized", RecommendedAction: "materialize the staging plan"}
		value, loadErr := s.repository.GetArtifact(ctx, plan.ArtifactID)
		if loadErr != nil {
			entry.ArtifactStatus = "unknown"
			entry.VerificationStatus = "error"
			entry.RecommendedAction = "restore or remove the invalid staging plan"
			result.Issues = append(result.Issues, readinessIssue("artifact_load_failed", loadErr.Error(), id, plan.ArtifactID))
		} else {
			entry.ArtifactStatus = string(value.Status)
			if value.Status != artifact.StatusApproved {
				entry.RecommendedAction = "approve the artifact or replace the staging plan"
				result.Issues = append(result.Issues, readinessIssue("artifact_not_approved", fmt.Sprintf("artifact status %q is not approved", value.Status), id, value.ID))
			} else if payloadErr := verifyArtifactPayload(ctx, s.payloadVerifier, value); payloadErr != nil {
				entry.RecommendedAction = "restore and verify the artifact payload"
				result.Issues = append(result.Issues, readinessIssue("payload_verification_failed", payloadErr.Error(), id, value.ID))
			} else if plan.ArtifactHash != value.SHA256 {
				entry.RecommendedAction = "create a new staging plan for the current artifact payload"
				result.Issues = append(result.Issues, readinessIssue("staging_plan_payload_mismatch", "staging plan payload hash does not match current artifact metadata", id, value.ID))
			} else {
				controllerReady[id] = true
			}
		}
		entries[id] = entry
	}

	if s.materializationVerifier == nil {
		result.Status = "error"
		result.Issues = append(result.Issues, readinessIssue("agent_verification_unavailable", "Agent materialization verification is unavailable", "", ""))
		return finishReadiness(result, entries), nil
	}
	verification, verifyErr := s.materializationVerifier.VerifyMaterializedArtifacts(ctx, sessionID)
	if verifyErr != nil {
		result.Status = "error"
		result.Issues = append(result.Issues, readinessIssue("agent_verification_failed", verifyErr.Error(), "", ""))
		return finishReadiness(result, entries), nil
	}
	result.MaterializedCount = verification.Total
	result.ValidMaterializedCount = verification.ValidCount
	result.MissingMaterializedCount = verification.MissingCount
	result.CorruptedMaterializedCount = verification.CorruptedCount
	seen := make(map[string]struct{}, len(verification.Entries))
	for _, verified := range verification.Entries {
		plan, known := planned[verified.StagingPlanID]
		if !known {
			result.UnknownMaterializedCount++
			result.Issues = append(result.Issues, readinessIssue("unknown_materialized_artifact", "materialized entry does not match a planned staging entry", verified.StagingPlanID, verified.ArtifactID))
			continue
		}
		seen[verified.StagingPlanID] = struct{}{}
		entry := entries[verified.StagingPlanID]
		entry.Materialized = true
		entry.VerificationStatus = verified.Status
		if verified.ArtifactID != plan.ArtifactID {
			entry.VerificationStatus = "error"
			entry.RecommendedAction = "inspect the Agent materialization manifest"
			result.Issues = append(result.Issues, readinessIssue("materialized_artifact_mismatch", "materialized entry artifact does not match its staging plan", verified.StagingPlanID, verified.ArtifactID))
			entries[verified.StagingPlanID] = entry
			continue
		}
		switch verified.Status {
		case "valid":
			if controllerReady[verified.StagingPlanID] {
				entry.RecommendedAction = "none"
			}
		case "missing":
			entry.RecommendedAction = "materialize the staging plan again"
			result.Issues = append(result.Issues, readinessIssue("materialized_file_missing", "materialized artifact file is missing", verified.StagingPlanID, plan.ArtifactID))
		case "corrupted":
			entry.RecommendedAction = "replace the corrupted materialized file through a future repair flow"
			result.Issues = append(result.Issues, readinessIssue("materialized_file_corrupted", "materialized artifact file failed hash or size verification", verified.StagingPlanID, plan.ArtifactID))
		default:
			entry.RecommendedAction = "inspect the Agent materialization manifest"
			message := verified.ErrorMessage
			if message == "" {
				message = "materialized artifact verification failed"
			}
			result.Issues = append(result.Issues, readinessIssue("materialized_verification_error", message, verified.StagingPlanID, plan.ArtifactID))
		}
		entries[verified.StagingPlanID] = entry
	}
	for id, plan := range planned {
		if _, ok := seen[id]; ok {
			continue
		}
		result.MissingMaterializedCount++
		result.Issues = append(result.Issues, readinessIssue("staging_plan_not_materialized", "planned artifact has no Agent materialization entry", id, plan.ArtifactID))
	}
	return finishReadiness(result, entries), nil
}

func finishReadiness(result ReadinessResult, entries map[string]ReadinessEntry) ReadinessResult {
	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		result.Entries = append(result.Entries, entries[id])
	}
	if result.Status != "error" && len(result.Issues) > 0 {
		result.Status = "not_ready"
	}
	return result
}

func readinessIssue(code, message, planID, artifactID string) ReadinessIssue {
	return ReadinessIssue{Code: code, Message: message, StagingPlanID: planID, ArtifactID: artifactID, Severity: "error"}
}
