package artifactstagingsvc

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/stratummc/stratum/internal/agent"
)

type AppliedArtifactVerifier interface {
	ListAppliedArtifacts(context.Context, string) (agent.AppliedArtifactsResponse, error)
	VerifyAllAppliedArtifacts(context.Context, string) (agent.BatchAppliedArtifactVerification, error)
}

type PreStartResult struct {
	Status                 string
	StagingReadinessStatus string
	AppliedVerifyStatus    string
	TotalApplied           int
	ValidApplied           int
	MissingApplied         int
	CorruptedApplied       int
	Issues                 []string
}

func (r PreStartResult) Metadata() map[string]string {
	return map[string]string{
		"artifactCheckEnabled":    "true",
		"stagingReadinessStatus":  r.StagingReadinessStatus,
		"appliedVerifyStatus":     r.AppliedVerifyStatus,
		"totalApplied":            strconv.Itoa(r.TotalApplied),
		"validApplied":            strconv.Itoa(r.ValidApplied),
		"missingApplied":          strconv.Itoa(r.MissingApplied),
		"corruptedApplied":        strconv.Itoa(r.CorruptedApplied),
		"artifactReadinessIssues": strings.Join(r.Issues, ","),
	}
}

type PreStartService struct {
	repository      ReadinessRepository
	payloadVerifier PayloadVerifier
	agent           interface {
		MaterializationVerifier
		AppliedArtifactVerifier
	}
}

func NewPreStartService(repository ReadinessRepository, payloadVerifier PayloadVerifier, agentClient interface {
	MaterializationVerifier
	AppliedArtifactVerifier
}) *PreStartService {
	return &PreStartService{repository: repository, payloadVerifier: payloadVerifier, agent: agentClient}
}

func (s *PreStartService) Check(ctx context.Context, sessionID string) (PreStartResult, error) {
	result := PreStartResult{Status: "ready", StagingReadinessStatus: "not_applicable", AppliedVerifyStatus: "not_applicable", Issues: []string{}}
	plans, err := s.repository.ListArtifactStagingPlansBySession(ctx, sessionID)
	if err != nil {
		return preStartError(result, "staging_plans_unavailable", fmt.Errorf("list artifact staging plans: %w", err))
	}
	if len(plans) > 0 {
		readiness, readinessErr := NewReadinessService(s.repository, s.payloadVerifier, s.agent).Check(ctx, sessionID)
		if readinessErr != nil {
			return preStartError(result, "staging_readiness_error", readinessErr)
		}
		result.StagingReadinessStatus = readiness.Status
		for _, issue := range readiness.Issues {
			result.Issues = append(result.Issues, issue.Code)
		}
		if readiness.Status != "ready" {
			result.Status = "not_ready"
		}
	}

	applied, err := s.agent.ListAppliedArtifacts(ctx, sessionID)
	if err != nil {
		return preStartError(result, "applied_manifest_unavailable", fmt.Errorf("list applied artifacts: %w", err))
	}
	if len(applied.Records) > 0 {
		verified, verifyErr := s.agent.VerifyAllAppliedArtifacts(ctx, sessionID)
		if verifyErr != nil {
			return preStartError(result, "applied_verification_error", fmt.Errorf("verify applied artifacts: %w", verifyErr))
		}
		result.TotalApplied = verified.Total
		result.ValidApplied = verified.ValidCount
		result.MissingApplied = verified.MissingCount
		result.CorruptedApplied = verified.CorruptedCount
		result.AppliedVerifyStatus = "valid"
		if verified.ValidCount != verified.Total || verified.MissingCount > 0 || verified.CorruptedCount > 0 || verified.ErrorCount > 0 {
			result.Status = "not_ready"
			result.AppliedVerifyStatus = "not_ready"
			if verified.MissingCount > 0 {
				result.Issues = append(result.Issues, "applied_artifact_missing")
			}
			if verified.CorruptedCount > 0 {
				result.Issues = append(result.Issues, "applied_artifact_corrupted")
			}
			if verified.ErrorCount > 0 {
				result.Issues = append(result.Issues, "applied_artifact_verification_error")
			}
		}
	}
	if result.Status != "ready" {
		return result, fmt.Errorf("artifact readiness gate blocked session start: %s", strings.Join(result.Issues, ", "))
	}
	return result, nil
}

func preStartError(result PreStartResult, issue string, err error) (PreStartResult, error) {
	result.Status = "error"
	result.Issues = append(result.Issues, issue)
	return result, fmt.Errorf("artifact readiness gate failed: %w", err)
}
