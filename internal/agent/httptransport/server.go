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
	s.mux.HandleFunc("POST /v1/checkpoints/create-stub", s.checkpointStub(true))
	s.mux.HandleFunc("POST /v1/checkpoints/restore-stub", s.checkpointStub(false))
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
	writeJSON(w, http.StatusOK, SessionInspectResponse{AgentID: status.AgentID, SessionID: status.SessionID, Status: status.Status, Running: status.Running, Frozen: status.Frozen, RuntimeEndpoint: status.RuntimeEndpoint, ProcessID: status.ProcessID, PID: status.PID, RuntimeMode: status.RuntimeMode, RuntimeProfileID: status.RuntimeProfileID, RuntimeType: status.RuntimeType, Crashed: status.Crashed, StartedAt: status.StartedAt, StoppedAt: status.StoppedAt, ExitCode: status.ExitCode, LastError: status.LastError, ObservedAt: status.ObservedAt, RequestID: requestID(r)})
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
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
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

func newRequestID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "request-unknown"
	}
	return hex.EncodeToString(bytes)
}
