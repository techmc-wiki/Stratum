package artifactstagingsvc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/domain/artifact"
)

type preStartAgent struct {
	materialized agent.MaterializedArtifactsVerification
	applied      agent.AppliedArtifactsResponse
	verified     agent.BatchAppliedArtifactVerification
	err          error
}

func (a preStartAgent) VerifyMaterializedArtifacts(context.Context, string) (agent.MaterializedArtifactsVerification, error) {
	if a.err != nil {
		return agent.MaterializedArtifactsVerification{}, a.err
	}
	return a.materialized, nil
}

func (a preStartAgent) ListAppliedArtifacts(context.Context, string) (agent.AppliedArtifactsResponse, error) {
	if a.err != nil {
		return agent.AppliedArtifactsResponse{}, a.err
	}
	return a.applied, nil
}

func (a preStartAgent) VerifyAllAppliedArtifacts(context.Context, string) (agent.BatchAppliedArtifactVerification, error) {
	if a.err != nil {
		return agent.BatchAppliedArtifactVerification{}, a.err
	}
	return a.verified, nil
}

func TestPreStartReadinessAllowsNoArtifacts(t *testing.T) {
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	result, err := NewPreStartService(store, matchingVerifier(store), preStartAgent{}).Check(context.Background(), "session-1")
	if err != nil || result.Status != "ready" || result.StagingReadinessStatus != "not_applicable" || result.AppliedVerifyStatus != "not_applicable" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestPreStartReadinessBlocksUnmaterializedStaging(t *testing.T) {
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	addPlannedArtifact(store)
	result, err := NewPreStartService(store, matchingVerifier(store), preStartAgent{}).Check(context.Background(), "session-1")
	if err == nil || result.Status != "not_ready" || result.StagingReadinessStatus != "not_ready" || !containsString(result.Issues, "staging_plan_not_materialized") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestPreStartReadinessAllowsReadyStagingAndApplied(t *testing.T) {
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	addPlannedArtifact(store)
	agentClient := preStartAgent{
		materialized: agent.MaterializedArtifactsVerification{SessionID: "session-1", Total: 1, ValidCount: 1, Entries: []agent.MaterializedArtifactVerification{{StagingPlanID: "plan-1", ArtifactID: "artifact-1", Status: "valid"}}},
		applied:      agent.AppliedArtifactsResponse{SessionID: "session-1", Records: []agent.AppliedArtifactRecord{{ApplyPlanID: "apply-1"}}},
		verified:     agent.BatchAppliedArtifactVerification{SessionID: "session-1", Total: 1, ValidCount: 1, Entries: []agent.AppliedArtifactVerification{{ApplyPlanID: "apply-1", Status: "valid"}}},
	}
	result, err := NewPreStartService(store, matchingVerifier(store), agentClient).Check(context.Background(), "session-1")
	if err != nil || result.Status != "ready" || result.StagingReadinessStatus != "ready" || result.AppliedVerifyStatus != "valid" || result.TotalApplied != 1 || result.ValidApplied != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestPreStartReadinessBlocksMissingAndCorruptedApplied(t *testing.T) {
	for _, test := range []struct {
		name      string
		verified  agent.BatchAppliedArtifactVerification
		issueCode string
	}{
		{name: "missing", verified: agent.BatchAppliedArtifactVerification{Total: 1, MissingCount: 1}, issueCode: "applied_artifact_missing"},
		{name: "corrupted", verified: agent.BatchAppliedArtifactVerification{Total: 1, CorruptedCount: 1}, issueCode: "applied_artifact_corrupted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
			agentClient := preStartAgent{applied: agent.AppliedArtifactsResponse{Records: []agent.AppliedArtifactRecord{{ApplyPlanID: "apply-1"}}}, verified: test.verified}
			result, err := NewPreStartService(store, matchingVerifier(store), agentClient).Check(context.Background(), "session-1")
			if err == nil || result.Status != "not_ready" || result.AppliedVerifyStatus != "not_ready" || !containsString(result.Issues, test.issueCode) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestPreStartReadinessBlocksAgentFailure(t *testing.T) {
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	result, err := NewPreStartService(store, matchingVerifier(store), preStartAgent{err: errors.New("agent unavailable")}).Check(context.Background(), "session-1")
	if err == nil || result.Status != "error" || !strings.Contains(err.Error(), "agent unavailable") || !containsString(result.Issues, "applied_manifest_unavailable") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
