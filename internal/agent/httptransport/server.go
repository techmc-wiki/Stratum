package httptransport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/stratummc/stratum/internal/agent"
)

const requestIDHeader = "X-Request-ID"

type Server struct {
	client agent.AgentClient
	token  string
	logger *log.Logger
	mux    *http.ServeMux
}

func NewServer(client agent.AgentClient, token string, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	server := &Server{client: client, token: token, logger: logger, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.withRequestID(s.withAuth(s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.health)
	s.mux.HandleFunc("GET /v1/agent", s.agentInfo)
	s.mux.HandleFunc("GET /v1/agent/resources", s.resources)
	s.mux.HandleFunc("GET /v1/agent/runtime-profiles", s.runtimeProfiles)
	for _, operation := range []string{"prepare", "start", "stop", "restart", "freeze", "unfreeze"} {
		s.mux.HandleFunc("POST /v1/sessions/{id}/"+operation, s.sessionOperation(operation))
	}
	s.mux.HandleFunc("GET /v1/sessions/{id}/inspect", s.inspectSession)
	s.mux.HandleFunc("GET /v1/sessions/{id}/logs", s.logs)
	s.mux.HandleFunc("GET /v1/sessions/{id}/artifacts", s.materializedArtifacts)
	s.mux.HandleFunc("GET /v1/sessions/{id}/artifacts/verify", s.verifyMaterializedArtifacts)
	s.mux.HandleFunc("GET /v1/sessions/{id}/artifacts/{plan}", s.materializedArtifact)
	s.mux.HandleFunc("GET /v1/sessions/{id}/artifacts/{plan}/verify", s.verifyMaterializedArtifact)
	s.mux.HandleFunc("POST /v1/checkpoints/create-stub", s.checkpointStub(true))
	s.mux.HandleFunc("POST /v1/checkpoints/restore-stub", s.checkpointStub(false))
	s.mux.HandleFunc("POST /v1/artifacts/materialize", s.materializeArtifact)
	s.mux.HandleFunc("POST /v1/artifacts/apply/dry-run", s.dryRunArtifactApply)
	s.mux.HandleFunc("POST /v1/artifacts/apply/execute", s.executeArtifactApply)
	s.mux.HandleFunc("GET /v1/sessions/{id}/applied-artifacts", s.listAppliedArtifacts)
	s.mux.HandleFunc("GET /v1/sessions/{id}/applied-artifacts/verify", s.verifyAllAppliedArtifacts)
	s.mux.HandleFunc("GET /v1/sessions/{id}/applied-artifacts/{plan}", s.inspectAppliedArtifact)
	s.mux.HandleFunc("GET /v1/sessions/{id}/applied-artifacts/{plan}/verify", s.verifyAppliedArtifact)
	s.mux.HandleFunc("POST /v1/environments/materialize", s.materializeEnvironment)
	s.mux.HandleFunc("GET /v1/sessions/{id}/runtime-status", s.sessionRuntimeStatus)
	s.mux.HandleFunc("GET /v1/sessions/{id}/ready-for-start", s.sessionReadyForStart)
	s.mux.HandleFunc("GET /v1/sessions/{id}/mcdr-config-stub", s.inspectMCDRConfigStub)
}

func (s *Server) verifyMaterializedArtifacts(w http.ResponseWriter, r *http.Request) {
	result, err := s.client.VerifyMaterializedArtifacts(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "verify-materialized-artifacts", err)
		return
	}
	entries := make([]MaterializedArtifactVerificationResponse, 0, len(result.Entries))
	for _, entry := range result.Entries {
		entries = append(entries, materializedArtifactVerificationResponse(entry))
	}
	writeJSON(w, http.StatusOK, MaterializedArtifactsVerificationResponse{AgentID: result.AgentID, SessionID: result.SessionID, VerifiedAt: result.VerifiedAt, Total: result.Total, ValidCount: result.ValidCount, MissingCount: result.MissingCount, CorruptedCount: result.CorruptedCount, ErrorCount: result.ErrorCount, Entries: entries, RequestID: requestID(r)})
}

func (s *Server) verifyMaterializedArtifact(w http.ResponseWriter, r *http.Request) {
	result, err := s.client.VerifyMaterializedArtifact(r.Context(), r.PathValue("id"), r.PathValue("plan"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
			status = http.StatusNotFound
		}
		s.writeError(w, r, status, "verify-materialized-artifact", err)
		return
	}
	response := materializedArtifactVerificationResponse(result)
	response.RequestID = requestID(r)
	writeJSON(w, http.StatusOK, response)
}

func materializedArtifactVerificationResponse(result agent.MaterializedArtifactVerification) MaterializedArtifactVerificationResponse {
	return MaterializedArtifactVerificationResponse{AgentID: result.AgentID, SessionID: result.SessionID, StagingPlanID: result.StagingPlanID, ArtifactID: result.ArtifactID, TargetName: result.TargetName, RuntimeRelativePath: result.RuntimeRelativePath, PayloadAlgorithm: result.PayloadAlgorithm, ExpectedHash: result.ExpectedHash, ActualHash: result.ActualHash, PayloadSize: result.PayloadSize, ActualSize: result.ActualSize, Status: result.Status, VerifiedAt: result.VerifiedAt, ErrorMessage: result.ErrorMessage}
}

func (s *Server) materializedArtifact(w http.ResponseWriter, r *http.Request) {
	item, err := s.client.InspectMaterializedArtifact(r.Context(), r.PathValue("id"), r.PathValue("plan"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
			status = http.StatusNotFound
		}
		s.writeError(w, r, status, "inspect-materialized-artifact", err)
		return
	}
	writeJSON(w, http.StatusOK, MaterializedArtifactResponse{AgentID: item.AgentID, SessionID: item.SessionID, ArtifactID: item.ArtifactID, StagingPlanID: item.StagingPlanID, ArtifactName: item.ArtifactName, ArtifactType: item.ArtifactType, TargetName: item.TargetName, PayloadAlgorithm: item.PayloadAlgorithm, PayloadHash: item.PayloadHash, PayloadSize: item.PayloadSize, RuntimeRelativePath: item.RuntimeRelativePath, MaterializedAt: item.MaterializedAt, ActorID: item.ActorID, Status: item.Status, Metadata: item.Metadata, RequestID: requestID(r)})
}

func (s *Server) materializedArtifacts(w http.ResponseWriter, r *http.Request) {
	result, err := s.client.InspectMaterializedArtifacts(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "inspect-materialized-artifacts", err)
		return
	}
	items := make([]MaterializedArtifactResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, MaterializedArtifactResponse{ArtifactID: item.ArtifactID, StagingPlanID: item.StagingPlanID, ArtifactName: item.ArtifactName, ArtifactType: item.ArtifactType, TargetName: item.TargetName, PayloadAlgorithm: item.PayloadAlgorithm, PayloadHash: item.PayloadHash, PayloadSize: item.PayloadSize, RuntimeRelativePath: item.RuntimeRelativePath, MaterializedAt: item.MaterializedAt, ActorID: item.ActorID, Status: item.Status, Metadata: item.Metadata})
	}
	writeJSON(w, http.StatusOK, MaterializedArtifactsResponse{AgentID: result.AgentID, SessionID: result.SessionID, Status: result.Status, Items: items, RequestID: requestID(r)})
}

func (s *Server) materializeArtifact(w http.ResponseWriter, r *http.Request) {
	var body ArtifactMaterializationRequest
	if err := decodeJSONLimited(r, &body, (agent.MaxArtifactPayloadBytes*4/3)+(2<<20)); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "materialize-artifact", err)
		return
	}
	request := agent.ArtifactMaterializationRequest{SessionID: body.SessionID, ArtifactID: body.ArtifactID, StagingPlanID: body.StagingPlanID, ArtifactName: body.ArtifactName, ArtifactType: body.ArtifactType, TargetName: body.TargetName, PayloadAlgorithm: body.PayloadAlgorithm, PayloadHash: body.PayloadHash, PayloadSize: body.PayloadSize, ActorID: body.ActorID, Payload: body.Payload}
	result, err := s.client.MaterializeArtifact(r.Context(), request)
	if err != nil {
		s.writeError(w, r, http.StatusConflict, "materialize-artifact", err)
		return
	}
	writeJSON(w, http.StatusOK, ArtifactMaterializationResponse{AgentID: result.AgentID, SessionID: result.SessionID, ArtifactID: result.ArtifactID, StagingPlanID: result.StagingPlanID, TargetName: result.TargetName, RuntimeRelativePath: result.RuntimeRelativePath, PayloadHash: result.PayloadHash, PayloadSize: result.PayloadSize, MaterializedAt: result.MaterializedAt, Idempotent: result.Idempotent, Status: result.Status, RequestID: requestID(r)})
}

func (s *Server) runtimeProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.client.RuntimeProfiles(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusBadGateway, "runtime-profiles", err)
		return
	}
	info, err := s.client.Info(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusBadGateway, "agent-info", err)
		return
	}
	writeJSON(w, http.StatusOK, RuntimeProfilesResponse{AgentID: info.ID, Profiles: profiles, RequestID: requestID(r)})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "requestId": requestID(r)})
}

func (s *Server) agentInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.client.Info(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusBadGateway, "agent-info", err)
		return
	}
	writeJSON(w, http.StatusOK, AgentInfoResponse{ID: info.ID, Status: info.Status, RuntimeEndpoint: info.RuntimeEndpoint, Capabilities: info.Capabilities, RequestID: requestID(r)})
}

func (s *Server) resources(w http.ResponseWriter, r *http.Request) {
	report, err := s.client.ReportResources(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusBadGateway, "report-resources", err)
		return
	}
	writeJSON(w, http.StatusOK, ResourceReportResponse{AgentID: report.AgentID, CPUCapacity: report.CPUCapacity, MemoryTotalMB: report.MemoryTotalMB, MemoryUsedMB: report.MemoryUsedMB, DiskTotalMB: report.DiskTotalMB, DiskUsedMB: report.DiskUsedMB, RunningSessions: report.RunningSessions, ReportedAt: report.ReportedAt, RequestID: requestID(r)})
}

func (s *Server) sessionOperation(operation string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body SessionOperationRequest
		if err := decodeJSON(r, &body); err != nil {
			s.writeError(w, r, http.StatusBadRequest, operation, err)
			return
		}
		request := agent.SessionRequest{SessionID: r.PathValue("id"), ProjectID: body.ProjectID, EnvironmentID: body.EnvironmentID, RuntimeProfileID: body.RuntimeProfileID}
		var result agent.OperationResult
		var err error
		switch operation {
		case "prepare":
			result, err = s.client.PrepareSession(r.Context(), request)
		case "start":
			result, err = s.client.StartSession(r.Context(), request)
		case "stop":
			result, err = s.client.StopSession(r.Context(), request)
		case "restart":
			result, err = s.client.RestartSession(r.Context(), request)
		case "freeze":
			result, err = s.client.FreezeSession(r.Context(), request)
		case "unfreeze":
			result, err = s.client.UnfreezeSession(r.Context(), request)
		}
		if err != nil {
			s.writeError(w, r, http.StatusConflict, operation, err)
			return
		}
		writeJSON(w, http.StatusOK, operationResponse(result, requestID(r)))
	}
}

func (s *Server) inspectSession(w http.ResponseWriter, r *http.Request) {
	status, err := s.client.InspectSession(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, http.StatusBadGateway, "inspect", err)
		return
	}
	writeJSON(w, http.StatusOK, SessionInspectResponse{AgentID: status.AgentID, SessionID: status.SessionID, Status: status.Status, Running: status.Running, Frozen: status.Frozen, RuntimeEndpoint: status.RuntimeEndpoint, ProcessID: status.ProcessID, PID: status.PID, RuntimeMode: status.RuntimeMode, RuntimeProfileID: status.RuntimeProfileID, RuntimeType: status.RuntimeType, Crashed: status.Crashed, StartedAt: status.StartedAt, StoppedAt: status.StoppedAt, ExitCode: status.ExitCode, LastError: status.LastError, ObservedAt: status.ObservedAt, SessionRoot: status.SessionRoot, WorkDir: status.WorkDir, LogsDir: status.LogsDir, RequestID: requestID(r)})
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	maxBytes := 0
	if raw := r.URL.Query().Get("maxBytes"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			s.writeError(w, r, http.StatusBadRequest, "collect-logs", errors.New("maxBytes must be a non-negative integer"))
			return
		}
		maxBytes = value
	}
	batch, err := s.client.CollectLogs(agent.WithLogMaxBytes(r.Context(), maxBytes), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, http.StatusBadGateway, "collect-logs", err)
		return
	}
	writeJSON(w, http.StatusOK, LogsResponse{AgentID: batch.AgentID, SessionID: batch.SessionID, Lines: batch.Lines, RequestID: requestID(r)})
}

func (s *Server) checkpointStub(create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body CheckpointStubRequest
		if err := decodeJSON(r, &body); err != nil {
			s.writeError(w, r, http.StatusBadRequest, "checkpoint-stub", err)
			return
		}
		request := agent.CheckpointRequest{SessionID: body.SessionID, CheckpointID: body.CheckpointID}
		var result agent.OperationResult
		var err error
		operation := "restore-checkpoint"
		if create {
			operation = "create-checkpoint"
			result, err = s.client.CreateCheckpointStub(r.Context(), request)
		} else {
			result, err = s.client.RestoreCheckpointStub(r.Context(), request)
		}
		if err != nil {
			s.writeError(w, r, http.StatusConflict, operation, err)
			return
		}
		writeJSON(w, http.StatusOK, operationResponse(result, requestID(r)))
	}
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && r.Header.Get("Authorization") != "Bearer "+s.token {
			s.writeError(w, r, http.StatusUnauthorized, "authenticate", errors.New("missing or invalid bearer token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if id == "" {
			id = newRequestID()
		}
		r.Header.Set(requestIDHeader, id)
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, operation string, err error) {
	agentID := ""
	var agentErr agent.Error
	if errors.As(err, &agentErr) {
		agentID = agentErr.AgentID
	}
	s.logger.Printf("request_id=%s operation=%s error=%q", requestID(r), operation, err.Error())
	writeJSON(w, status, ErrorResponse{Error: err.Error(), Operation: operation, AgentID: agentID, RequestID: requestID(r)})
}

func decodeJSON(r *http.Request, target any) error {
	return decodeJSONLimited(r, target, 1<<20)
}

func decodeJSONLimited(r *http.Request, target any, limit int64) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func operationResponse(result agent.OperationResult, requestID string) SessionOperationResponse {
	return SessionOperationResponse{AgentID: result.AgentID, Status: result.Status, Message: result.Message, RequestID: requestID}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requestID(r *http.Request) string { return r.Header.Get(requestIDHeader) }

func (s *Server) dryRunArtifactApply(w http.ResponseWriter, r *http.Request) {
	var dto ArtifactApplyDryRunRequestDTO
	if err := decodeJSONLimited(r, &dto, 4096); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "dry-run-artifact-apply", err)
		return
	}
	req := agent.ArtifactApplyDryRunRequest{ApplyPlanID: dto.ApplyPlanID, SessionID: dto.SessionID, StagingPlanID: dto.StagingPlanID, ArtifactID: dto.ArtifactID, TargetRoot: dto.TargetRoot, TargetRelativePath: dto.TargetRelativePath, ExpectedHash: dto.ExpectedHash, ExpectedSize: dto.ExpectedSize}
	result, err := s.client.DryRunArtifactApply(r.Context(), req)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "dry-run-artifact-apply", err)
		return
	}
	writeJSON(w, http.StatusOK, ArtifactApplyDryRunResultDTO{AgentID: result.AgentID, ApplyPlanID: result.ApplyPlanID, SessionID: result.SessionID, ArtifactID: result.ArtifactID, StagingPlanID: result.StagingPlanID, ApplyKind: result.ApplyKind, TargetRoot: result.TargetRoot, TargetRelativePath: result.TargetRelativePath, SourceRuntimeRelativePath: result.SourceRuntimeRelativePath, PlannedTargetRuntimeRelativePath: result.PlannedTargetRuntimeRelativePath, Action: result.Action, Status: result.Status, Issues: result.Issues, CheckedAt: result.CheckedAt, RequestID: requestID(r)})
}

func (s *Server) executeArtifactApply(w http.ResponseWriter, r *http.Request) {
	var dto ArtifactApplyExecuteRequestDTO
	if err := decodeJSONLimited(r, &dto, 4096); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "execute-artifact-apply", err)
		return
	}
	req := agent.ArtifactApplyExecuteRequest{ApplyPlanID: dto.ApplyPlanID, SessionID: dto.SessionID, StagingPlanID: dto.StagingPlanID, ArtifactID: dto.ArtifactID, TargetRoot: dto.TargetRoot, TargetRelativePath: dto.TargetRelativePath, ExpectedHash: dto.ExpectedHash, ExpectedSize: dto.ExpectedSize}
	result, err := s.client.ExecuteArtifactApply(r.Context(), req)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "execute-artifact-apply", err)
		return
	}
	writeJSON(w, http.StatusOK, ArtifactApplyExecuteResultDTO{AgentID: result.AgentID, ApplyPlanID: result.ApplyPlanID, SessionID: result.SessionID, ArtifactID: result.ArtifactID, StagingPlanID: result.StagingPlanID, TargetRoot: result.TargetRoot, TargetRelativePath: result.TargetRelativePath, SourcePath: result.SourcePath, TargetPath: result.TargetPath, Action: result.Action, Status: result.Status, Issues: result.Issues, CopiedBytes: result.CopiedBytes, VerifiedTargetHash: result.VerifiedTargetHash, ExecutedAt: result.ExecutedAt, RequestID: requestID(r)})
}

func (s *Server) listAppliedArtifacts(w http.ResponseWriter, r *http.Request) {
	result, err := s.client.ListAppliedArtifacts(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "list-applied-artifacts", err)
		return
	}
	records := make([]AppliedArtifactRecordDTO, 0, len(result.Records))
	for _, rec := range result.Records {
		records = append(records, AppliedArtifactRecordDTO{ApplyPlanID: rec.ApplyPlanID, SessionID: rec.SessionID, ArtifactID: rec.ArtifactID, StagingPlanID: rec.StagingPlanID, SourceRuntimeRelativePath: rec.SourceRuntimeRelativePath, TargetRuntimeRelativePath: rec.TargetRuntimeRelativePath, TargetRoot: rec.TargetRoot, TargetRelativePath: rec.TargetRelativePath, PayloadAlgorithm: rec.PayloadAlgorithm, PayloadHash: rec.PayloadHash, PayloadSize: rec.PayloadSize, Action: rec.Action, Status: rec.Status, ActorID: rec.ActorID, AppliedAt: rec.AppliedAt})
	}
	writeJSON(w, http.StatusOK, AppliedArtifactsResponse{SessionID: result.SessionID, Records: records, RequestID: requestID(r)})
}

func (s *Server) inspectAppliedArtifact(w http.ResponseWriter, r *http.Request) {
	result, err := s.client.InspectAppliedArtifact(r.Context(), r.PathValue("id"), r.PathValue("plan"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
			status = http.StatusNotFound
		}
		s.writeError(w, r, status, "inspect-applied-artifact", err)
		return
	}
	writeJSON(w, http.StatusOK, AppliedArtifactRecordDTO{ApplyPlanID: result.ApplyPlanID, SessionID: result.SessionID, ArtifactID: result.ArtifactID, StagingPlanID: result.StagingPlanID, SourceRuntimeRelativePath: result.SourceRuntimeRelativePath, TargetRuntimeRelativePath: result.TargetRuntimeRelativePath, TargetRoot: result.TargetRoot, TargetRelativePath: result.TargetRelativePath, PayloadAlgorithm: result.PayloadAlgorithm, PayloadHash: result.PayloadHash, PayloadSize: result.PayloadSize, Action: result.Action, Status: result.Status, ActorID: result.ActorID, AppliedAt: result.AppliedAt})
}

func (s *Server) verifyAppliedArtifact(w http.ResponseWriter, r *http.Request) {
	result, err := s.client.VerifyAppliedArtifact(r.Context(), r.PathValue("id"), r.PathValue("plan"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
			status = http.StatusNotFound
		}
		s.writeError(w, r, status, "verify-applied-artifact", err)
		return
	}
	writeJSON(w, http.StatusOK, AppliedArtifactVerificationDTO{SessionID: result.SessionID, ApplyPlanID: result.ApplyPlanID, ArtifactID: result.ArtifactID, StagingPlanID: result.StagingPlanID, TargetRoot: result.TargetRoot, TargetRelativePath: result.TargetRelativePath, TargetRuntimeRelativePath: result.TargetRuntimeRelativePath, PayloadAlgorithm: result.PayloadAlgorithm, ExpectedHash: result.ExpectedHash, ActualHash: result.ActualHash, PayloadSize: result.PayloadSize, ActualSize: result.ActualSize, Status: result.Status, VerifiedAt: result.VerifiedAt, ErrorMessage: result.ErrorMessage, RequestID: requestID(r)})
}

func (s *Server) verifyAllAppliedArtifacts(w http.ResponseWriter, r *http.Request) {
	result, err := s.client.VerifyAllAppliedArtifacts(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "verify-all-applied-artifacts", err)
		return
	}
	entries := make([]AppliedArtifactVerificationDTO, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = AppliedArtifactVerificationDTO{SessionID: e.SessionID, ApplyPlanID: e.ApplyPlanID, ArtifactID: e.ArtifactID, StagingPlanID: e.StagingPlanID, TargetRoot: e.TargetRoot, TargetRelativePath: e.TargetRelativePath, TargetRuntimeRelativePath: e.TargetRuntimeRelativePath, PayloadAlgorithm: e.PayloadAlgorithm, ExpectedHash: e.ExpectedHash, ActualHash: e.ActualHash, PayloadSize: e.PayloadSize, ActualSize: e.ActualSize, Status: e.Status, VerifiedAt: e.VerifiedAt, ErrorMessage: e.ErrorMessage}
	}
	writeJSON(w, http.StatusOK, BatchAppliedArtifactVerificationDTO{SessionID: result.SessionID, VerifiedAt: result.VerifiedAt, Total: result.Total, ValidCount: result.ValidCount, MissingCount: result.MissingCount, CorruptedCount: result.CorruptedCount, ErrorCount: result.ErrorCount, Entries: entries, RequestID: requestID(r)})
}

func (s *Server) materializeEnvironment(w http.ResponseWriter, r *http.Request) {
	var dto EnvironmentMaterializationRequest
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "materialize-environment", fmt.Errorf("decode request: %w", err))
		return
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:              dto.SessionID,
		EnvironmentID:          dto.EnvironmentID,
		EnvironmentName:        dto.EnvironmentName,
		MinecraftVersion:       dto.MinecraftVersion,
		JavaVersion:            dto.JavaVersion,
		LoaderType:             dto.LoaderType,
		LoaderVersion:          dto.LoaderVersion,
		ServerCore:             dto.ServerCore,
		MCDRRequired:           dto.MCDRRequired,
		CarpetRequired:         dto.CarpetRequired,
		RuntimeProfileID:       dto.RuntimeProfileID,
		RuntimeProfileRequired: dto.RuntimeProfileRequired,
		ActorID:                dto.ActorID,
	}
	result, err := s.client.MaterializeEnvironment(r.Context(), request)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "materialize-environment", err)
		return
	}
	writeJSON(w, http.StatusOK, EnvironmentMaterializationResponse{
		SessionID:              result.SessionID,
		EnvironmentID:          result.EnvironmentID,
		EnvironmentName:        result.EnvironmentName,
		MinecraftVersion:       result.MinecraftVersion,
		JavaVersion:            result.JavaVersion,
		LoaderType:             result.LoaderType,
		LoaderVersion:          result.LoaderVersion,
		ServerCore:             result.ServerCore,
		MCDRRequired:           result.MCDRRequired,
		CarpetRequired:         result.CarpetRequired,
		RuntimeProfileID:       result.RuntimeProfileID,
		RuntimeProfileRequired: result.RuntimeProfileRequired,
		MaterializedAt:         result.MaterializedAt,
		Status:                 result.Status,
		Directories:            result.Directories,
		Metadata:               result.Metadata,
		RequestID:              requestID(r),
	})
}

func (s *Server) sessionRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.client.GetSessionRuntimeStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "get-session-runtime-status", err)
		return
	}
	writeJSON(w, http.StatusOK, sessionRuntimeStatusResponse(status, requestID(r)))
}

func (s *Server) sessionReadyForStart(w http.ResponseWriter, r *http.Request) {
	result, err := s.client.SessionReadyForStart(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "session-ready-for-start", err)
		return
	}
	issues := make([]SessionStartReadinessIssue, len(result.Issues))
	for index, issue := range result.Issues {
		issues[index] = SessionStartReadinessIssue{Code: issue.Code, Message: issue.Message, Severity: issue.Severity}
	}
	summary := result.RuntimeStatusSummary
	writeJSON(w, http.StatusOK, SessionStartReadinessResponse{
		SessionID: result.SessionID, CheckedAt: result.CheckedAt, Ready: result.Ready, Status: result.Status, Issues: issues,
		RuntimeStatusSummary: SessionStartReadinessSummary{
			RuntimeRootExists: summary.RuntimeRootExists, SessionRootExists: summary.SessionRootExists,
			EnvironmentManifestExists: summary.EnvironmentManifestExists, EnvironmentManifestStatus: summary.EnvironmentManifestStatus,
			WorkDirExists: summary.WorkDirExists, ConfigDirExists: summary.ConfigDirExists, LogsDirExists: summary.LogsDirExists,
			ProcessState: summary.ProcessState, AppliedArtifactsTotal: summary.AppliedArtifactsTotal, AppliedArtifactsValid: summary.AppliedArtifactsValid,
			AppliedArtifactsMissing: summary.AppliedArtifactsMissing, AppliedArtifactsCorrupted: summary.AppliedArtifactsCorrupted, AppliedArtifactsError: summary.AppliedArtifactsError,
		}, RequestID: requestID(r),
	})
}

func (s *Server) inspectMCDRConfigStub(w http.ResponseWriter, r *http.Request) {
	result, err := s.client.InspectMCDRConfigStub(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "inspect-mcdr-config-stub", err)
		return
	}
	writeJSON(w, http.StatusOK, MCDRConfigStubInspectionDTO{
		SessionID:                   result.SessionID,
		Exists:                      result.Exists,
		Path:                        result.Path,
		Valid:                       result.Valid,
		Status:                      result.Status,
		PlannedConfigYMLPath:        result.PlannedConfigYMLPath,
		PlannedServerPropertiesPath: result.PlannedServerPropertiesPath,
		PlannedEULAPath:             result.PlannedEULAPath,
		Issues:                      result.Issues,
		CheckedAt:                   result.CheckedAt,
	})
}

func newRequestID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "request-unknown"
	}
	return hex.EncodeToString(bytes)
}
