package artifactapplysvc

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/artifactapply"
	"github.com/stratummc/stratum/internal/domain/artifactstaging"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/session"
	"github.com/stratummc/stratum/internal/util"
)

const ActionPlanCreated = "artifact.apply_plan.created"
const ActionPlanRejected = "artifact.apply_plan.rejected"

type Repository interface {
	GetSession(context.Context, string) (session.Session, error)
	GetArtifact(context.Context, string) (artifact.Artifact, error)
	GetArtifactStagingPlan(context.Context, string) (artifactstaging.Plan, error)
	CreateArtifactApplyPlan(context.Context, artifactapply.Plan) error
	GetArtifactApplyPlan(context.Context, string) (artifactapply.Plan, error)
	ListArtifactApplyPlans(context.Context) ([]artifactapply.Plan, error)
	ListArtifactApplyPlansBySession(context.Context, string) ([]artifactapply.Plan, error)
	AppendAuditEvent(context.Context, audit.Event) error
}

type MaterializationVerifier interface {
	VerifyMaterializedArtifacts(context.Context, string) (agent.MaterializedArtifactsVerification, error)
}

type CreateParams struct {
	SessionID     string
	StagingPlanID string
	ActorID       string
	TargetPath    string
}

type Service struct {
	repository              Repository
	materializationVerifier MaterializationVerifier
	now                     func() time.Time
	newID                   func(string) (string, error)
}

func New(repository Repository, verifier MaterializationVerifier) *Service {
	return &Service{repository: repository, materializationVerifier: verifier, now: func() time.Time { return time.Now().UTC() }, newID: util.NewID}
}

func (s *Service) Get(ctx context.Context, id string) (artifactapply.Plan, error) {
	return s.repository.GetArtifactApplyPlan(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]artifactapply.Plan, error) {
	return s.repository.ListArtifactApplyPlans(ctx)
}

func (s *Service) ListBySession(ctx context.Context, sessionID string) ([]artifactapply.Plan, error) {
	return s.repository.ListArtifactApplyPlansBySession(ctx, sessionID)
}

func (s *Service) CreatePlan(ctx context.Context, params CreateParams) (artifactapply.Plan, error) {
	if strings.TrimSpace(params.SessionID) == "" || strings.TrimSpace(params.StagingPlanID) == "" || strings.TrimSpace(params.ActorID) == "" || strings.TrimSpace(params.TargetPath) == "" {
		return artifactapply.Plan{}, fmt.Errorf("session, staging plan, actor, and target path are required")
	}
	sess, err := s.repository.GetSession(ctx, params.SessionID)
	if err != nil {
		return artifactapply.Plan{}, fmt.Errorf("load session: %w", err)
	}
	stagingPlan, err := s.repository.GetArtifactStagingPlan(ctx, params.StagingPlanID)
	if err != nil {
		return artifactapply.Plan{}, fmt.Errorf("load staging plan: %w", err)
	}
	if stagingPlan.SessionID != params.SessionID {
		return artifactapply.Plan{}, fmt.Errorf("staging plan does not belong to session")
	}
	art, err := s.repository.GetArtifact(ctx, stagingPlan.ArtifactID)
	if err != nil {
		return artifactapply.Plan{}, fmt.Errorf("load artifact: %w", err)
	}
	if art.Status != artifact.StatusApproved {
		return s.reject(ctx, sess, stagingPlan, art, params, "artifact not approved")
	}
	if art.PayloadStatus != artifact.PayloadAvailable {
		return s.reject(ctx, sess, stagingPlan, art, params, "payload not verified")
	}
	verification, err := s.materializationVerifier.VerifyMaterializedArtifacts(ctx, params.SessionID)
	if err != nil {
		return artifactapply.Plan{}, fmt.Errorf("verify materialization: %w", err)
	}
	var mat *agent.MaterializedArtifactVerification
	for _, e := range verification.Entries {
		if e.StagingPlanID == params.StagingPlanID {
			mat = &e
			break
		}
	}
	if mat == nil {
		return s.reject(ctx, sess, stagingPlan, art, params, "staging plan not materialized")
	}
	if mat.Status != "valid" {
		return s.reject(ctx, sess, stagingPlan, art, params, "materialized artifact not valid")
	}
	target := filepath.Clean(params.TargetPath)
	if filepath.IsAbs(target) || strings.Contains(target, "..") || target == "." {
		return s.reject(ctx, sess, stagingPlan, art, params, "unsafe target path")
	}
	kind := mapKind(art.Type)
	root := mapRoot(kind)
	id, _ := s.newID("artifact-apply-plan")
	plan := artifactapply.Plan{ID: id, SessionID: params.SessionID, ProjectID: sess.ProjectID, ActorID: params.ActorID, SourceStagingPlanID: params.StagingPlanID, ArtifactID: art.ID, MaterializedArtifactHash: mat.ExpectedHash, MaterializedArtifactName: mat.TargetName, ApplyKind: kind, TargetRoot: root, TargetRelativePath: target, Status: artifactapply.StatusPlanned, ValidationStatus: "validated", CreatedAt: s.now()}
	_ = s.repository.CreateArtifactApplyPlan(ctx, plan)
	_ = s.audit(ctx, ActionPlanCreated, plan, "")
	return plan, nil
}

func (s *Service) reject(ctx context.Context, sess session.Session, sp artifactstaging.Plan, art artifact.Artifact, params CreateParams, reason string) (artifactapply.Plan, error) {
	id, _ := s.newID("artifact-apply-plan")
	target := filepath.Clean(params.TargetPath)
	kind := mapKind(art.Type)
	root := mapRoot(kind)
	plan := artifactapply.Plan{ID: id, SessionID: params.SessionID, ProjectID: sess.ProjectID, ActorID: params.ActorID, SourceStagingPlanID: params.StagingPlanID, ArtifactID: art.ID, MaterializedArtifactHash: art.SHA256, MaterializedArtifactName: sp.TargetStagingName, ApplyKind: kind, TargetRoot: root, TargetRelativePath: target, Status: artifactapply.StatusRejected, RejectionReason: reason, CreatedAt: s.now()}
	_ = s.repository.CreateArtifactApplyPlan(ctx, plan)
	_ = s.audit(ctx, ActionPlanRejected, plan, reason)
	return plan, nil
}

func (s *Service) audit(ctx context.Context, action string, plan artifactapply.Plan, extra string) error {
	id, _ := s.newID("audit")
	return s.repository.AppendAuditEvent(ctx, audit.Event{ID: id, ActorID: plan.ActorID, Action: action, TargetType: "artifact_apply_plan", TargetID: plan.ID, ProjectID: plan.ProjectID, Metadata: map[string]string{"planId": plan.ID, "sessionId": plan.SessionID, "artifactId": plan.ArtifactID, "status": string(plan.Status), "extra": extra}, CreatedAt: s.now()})
}

func mapKind(t artifact.Type) artifactapply.Kind {
	switch t {
	case artifact.TypeJar:
		return artifactapply.KindMod
	case artifact.TypeConfigPreset:
		return artifactapply.KindConfig
	case artifact.TypeDatapack:
		return artifactapply.KindDatapack
	case artifact.TypeMCDRPlugin:
		return artifactapply.KindMCDRPlugin
	case artifact.TypeSchematic:
		return artifactapply.KindSchematic
	case artifact.TypeWorldArchive:
		return artifactapply.KindWorldArchive
	default:
		return artifactapply.KindOther
	}
}

func mapRoot(k artifactapply.Kind) artifactapply.TargetRoot {
	switch k {
	case artifactapply.KindMod:
		return artifactapply.TargetRootMods
	case artifactapply.KindConfig:
		return artifactapply.TargetRootConfig
	case artifactapply.KindDatapack:
		return artifactapply.TargetRootDatapacks
	case artifactapply.KindMCDRPlugin:
		return artifactapply.TargetRootPlugins
	case artifactapply.KindSchematic:
		return artifactapply.TargetRootSchematics
	case artifactapply.KindWorldArchive:
		return artifactapply.TargetRootWorlds
	default:
		return artifactapply.TargetRootCustom
	}
}
