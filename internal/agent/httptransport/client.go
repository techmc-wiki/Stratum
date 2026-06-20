package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

type HTTPError struct {
	StatusCode int
	RequestID  string
	AgentID    string
	Operation  string
	Message    string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("agent HTTP %s failed with status %d (request %s): %s", e.Operation, e.StatusCode, e.RequestID, e.Message)
}

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

var _ agent.AgentClient = (*Client)(nil)

func NewClient(rawURL, token string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid agent URL %q", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("agent URL scheme must be http or https")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{baseURL: parsed, token: token, httpClient: &http.Client{Timeout: timeout}}, nil
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return agent.WithRequestID(ctx, requestID)
}

func (c *Client) Info(ctx context.Context) (agent.AgentInfo, error) {
	var response AgentInfoResponse
	if err := c.do(ctx, http.MethodGet, "/v1/agent", nil, &response); err != nil {
		return agent.AgentInfo{}, err
	}
	return agent.AgentInfo{ID: response.ID, Status: response.Status, RuntimeEndpoint: c.baseURL.String(), Capabilities: response.Capabilities, Mode: "http"}, nil
}

func (c *Client) RuntimeProfiles(ctx context.Context) ([]runtimeprofile.Profile, error) {
	var response RuntimeProfilesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/agent/runtime-profiles", nil, &response); err != nil {
		return nil, err
	}
	return response.Profiles, nil
}

func (c *Client) PrepareSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "prepare")
}

func (c *Client) StartSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "start")
}

func (c *Client) StopSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "stop")
}

func (c *Client) RestartSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "restart")
}

func (c *Client) FreezeSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "freeze")
}

func (c *Client) UnfreezeSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "unfreeze")
}

func (c *Client) InspectSession(ctx context.Context, sessionID string) (agent.SessionStatus, error) {
	var response SessionInspectResponse
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(sessionID)+"/inspect", nil, &response); err != nil {
		return agent.SessionStatus{}, err
	}
	return agent.SessionStatus{AgentID: response.AgentID, SessionID: response.SessionID, Status: response.Status, Running: response.Running, Frozen: response.Frozen, RuntimeEndpoint: response.RuntimeEndpoint, ProcessID: response.ProcessID, PID: response.PID, RuntimeMode: response.RuntimeMode, RuntimeProfileID: response.RuntimeProfileID, RuntimeType: response.RuntimeType, Crashed: response.Crashed, StartedAt: response.StartedAt, StoppedAt: response.StoppedAt, ExitCode: response.ExitCode, LastError: response.LastError, ObservedAt: response.ObservedAt, SessionRoot: response.SessionRoot, WorkDir: response.WorkDir, LogsDir: response.LogsDir}, nil
}

func (c *Client) CollectLogs(ctx context.Context, sessionID string) (agent.LogBatch, error) {
	var response LogsResponse
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/logs"
	if maxBytes := agent.LogMaxBytesFromContext(ctx); maxBytes > 0 {
		path += "?maxBytes=" + strconv.Itoa(maxBytes)
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return agent.LogBatch{}, err
	}
	return agent.LogBatch{AgentID: response.AgentID, SessionID: response.SessionID, Lines: response.Lines}, nil
}

func (c *Client) ReportResources(ctx context.Context) (agent.ResourceReport, error) {
	var response ResourceReportResponse
	if err := c.do(ctx, http.MethodGet, "/v1/agent/resources", nil, &response); err != nil {
		return agent.ResourceReport{}, err
	}
	return agent.ResourceReport{AgentID: response.AgentID, CPUCapacity: response.CPUCapacity, MemoryTotalMB: response.MemoryTotalMB, MemoryUsedMB: response.MemoryUsedMB, DiskTotalMB: response.DiskTotalMB, DiskUsedMB: response.DiskUsedMB, RunningSessions: response.RunningSessions, ReportedAt: response.ReportedAt}, nil
}

func (c *Client) CreateCheckpointStub(ctx context.Context, request agent.CheckpointRequest) (agent.OperationResult, error) {
	return c.checkpointOperation(ctx, request, "create-stub")
}

func (c *Client) RestoreCheckpointStub(ctx context.Context, request agent.CheckpointRequest) (agent.OperationResult, error) {
	return c.checkpointOperation(ctx, request, "restore-stub")
}

func (c *Client) MaterializeArtifact(ctx context.Context, request agent.ArtifactMaterializationRequest) (agent.ArtifactMaterializationResult, error) {
	body := ArtifactMaterializationRequest{SessionID: request.SessionID, ArtifactID: request.ArtifactID, StagingPlanID: request.StagingPlanID, ArtifactName: request.ArtifactName, ArtifactType: request.ArtifactType, TargetName: request.TargetName, PayloadAlgorithm: request.PayloadAlgorithm, PayloadHash: request.PayloadHash, PayloadSize: request.PayloadSize, ActorID: request.ActorID, Payload: request.Payload}
	var response ArtifactMaterializationResponse
	if err := c.do(ctx, http.MethodPost, "/v1/artifacts/materialize", body, &response); err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	return agent.ArtifactMaterializationResult{AgentID: response.AgentID, SessionID: response.SessionID, ArtifactID: response.ArtifactID, StagingPlanID: response.StagingPlanID, TargetName: response.TargetName, RuntimeRelativePath: response.RuntimeRelativePath, PayloadHash: response.PayloadHash, PayloadSize: response.PayloadSize, MaterializedAt: response.MaterializedAt, Idempotent: response.Idempotent, Status: response.Status}, nil
}

func (c *Client) InspectMaterializedArtifacts(ctx context.Context, sessionID string) (agent.MaterializedArtifacts, error) {
	var response MaterializedArtifactsResponse
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(sessionID)+"/artifacts", nil, &response); err != nil {
		return agent.MaterializedArtifacts{}, err
	}
	items := make([]agent.MaterializedArtifact, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, agent.MaterializedArtifact{ArtifactID: item.ArtifactID, StagingPlanID: item.StagingPlanID, ArtifactName: item.ArtifactName, ArtifactType: item.ArtifactType, TargetName: item.TargetName, PayloadAlgorithm: item.PayloadAlgorithm, PayloadHash: item.PayloadHash, PayloadSize: item.PayloadSize, RuntimeRelativePath: item.RuntimeRelativePath, MaterializedAt: item.MaterializedAt, ActorID: item.ActorID, Status: item.Status, Metadata: item.Metadata})
	}
	return agent.MaterializedArtifacts{AgentID: response.AgentID, SessionID: response.SessionID, Status: response.Status, Items: items}, nil
}

func (c *Client) InspectMaterializedArtifact(ctx context.Context, sessionID, stagingPlanID string) (agent.MaterializedArtifact, error) {
	var response MaterializedArtifactResponse
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/artifacts/" + url.PathEscape(stagingPlanID)
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return agent.MaterializedArtifact{}, err
	}
	return agent.MaterializedArtifact{AgentID: response.AgentID, SessionID: response.SessionID, ArtifactID: response.ArtifactID, StagingPlanID: response.StagingPlanID, ArtifactName: response.ArtifactName, ArtifactType: response.ArtifactType, TargetName: response.TargetName, PayloadAlgorithm: response.PayloadAlgorithm, PayloadHash: response.PayloadHash, PayloadSize: response.PayloadSize, RuntimeRelativePath: response.RuntimeRelativePath, MaterializedAt: response.MaterializedAt, ActorID: response.ActorID, Status: response.Status, Metadata: response.Metadata}, nil
}

func (c *Client) VerifyMaterializedArtifact(ctx context.Context, sessionID, stagingPlanID string) (agent.MaterializedArtifactVerification, error) {
	var response MaterializedArtifactVerificationResponse
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/artifacts/" + url.PathEscape(stagingPlanID) + "/verify"
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return agent.MaterializedArtifactVerification{}, err
	}
	return agent.MaterializedArtifactVerification{AgentID: response.AgentID, SessionID: response.SessionID, StagingPlanID: response.StagingPlanID, ArtifactID: response.ArtifactID, TargetName: response.TargetName, RuntimeRelativePath: response.RuntimeRelativePath, PayloadAlgorithm: response.PayloadAlgorithm, ExpectedHash: response.ExpectedHash, ActualHash: response.ActualHash, PayloadSize: response.PayloadSize, ActualSize: response.ActualSize, Status: response.Status, VerifiedAt: response.VerifiedAt}, nil
}

func (c *Client) VerifyMaterializedArtifacts(ctx context.Context, sessionID string) (agent.MaterializedArtifactsVerification, error) {
	var response MaterializedArtifactsVerificationResponse
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/artifacts/verify"
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return agent.MaterializedArtifactsVerification{}, err
	}
	entries := make([]agent.MaterializedArtifactVerification, 0, len(response.Entries))
	for _, entry := range response.Entries {
		entries = append(entries, agent.MaterializedArtifactVerification{AgentID: entry.AgentID, SessionID: entry.SessionID, StagingPlanID: entry.StagingPlanID, ArtifactID: entry.ArtifactID, TargetName: entry.TargetName, RuntimeRelativePath: entry.RuntimeRelativePath, PayloadAlgorithm: entry.PayloadAlgorithm, ExpectedHash: entry.ExpectedHash, ActualHash: entry.ActualHash, PayloadSize: entry.PayloadSize, ActualSize: entry.ActualSize, Status: entry.Status, VerifiedAt: entry.VerifiedAt, ErrorMessage: entry.ErrorMessage})
	}
	return agent.MaterializedArtifactsVerification{AgentID: response.AgentID, SessionID: response.SessionID, VerifiedAt: response.VerifiedAt, Total: response.Total, ValidCount: response.ValidCount, MissingCount: response.MissingCount, CorruptedCount: response.CorruptedCount, ErrorCount: response.ErrorCount, Entries: entries}, nil
}

func (c *Client) DryRunArtifactApply(ctx context.Context, req agent.ArtifactApplyDryRunRequest) (agent.ArtifactApplyDryRunResult, error) {
	var response ArtifactApplyDryRunResultDTO
	body := ArtifactApplyDryRunRequestDTO{ApplyPlanID: req.ApplyPlanID, SessionID: req.SessionID, StagingPlanID: req.StagingPlanID, ArtifactID: req.ArtifactID, TargetRoot: req.TargetRoot, TargetRelativePath: req.TargetRelativePath, ExpectedHash: req.ExpectedHash, ExpectedSize: req.ExpectedSize}
	if err := c.do(ctx, http.MethodPost, "/v1/artifacts/apply/dry-run", body, &response); err != nil {
		return agent.ArtifactApplyDryRunResult{}, err
	}
	return agent.ArtifactApplyDryRunResult{AgentID: response.AgentID, ApplyPlanID: response.ApplyPlanID, SessionID: response.SessionID, ArtifactID: response.ArtifactID, StagingPlanID: response.StagingPlanID, ApplyKind: response.ApplyKind, TargetRoot: response.TargetRoot, TargetRelativePath: response.TargetRelativePath, SourceRuntimeRelativePath: response.SourceRuntimeRelativePath, PlannedTargetRuntimeRelativePath: response.PlannedTargetRuntimeRelativePath, Action: response.Action, Status: response.Status, Issues: response.Issues, CheckedAt: response.CheckedAt}, nil
}

func (c *Client) ExecuteArtifactApply(ctx context.Context, req agent.ArtifactApplyExecuteRequest) (agent.ArtifactApplyExecuteResult, error) {
	var response ArtifactApplyExecuteResultDTO
	body := ArtifactApplyExecuteRequestDTO{ApplyPlanID: req.ApplyPlanID, SessionID: req.SessionID, StagingPlanID: req.StagingPlanID, ArtifactID: req.ArtifactID, TargetRoot: req.TargetRoot, TargetRelativePath: req.TargetRelativePath, ExpectedHash: req.ExpectedHash, ExpectedSize: req.ExpectedSize}
	if err := c.do(ctx, http.MethodPost, "/v1/artifacts/apply/execute", body, &response); err != nil {
		return agent.ArtifactApplyExecuteResult{}, err
	}
	return agent.ArtifactApplyExecuteResult{AgentID: response.AgentID, ApplyPlanID: response.ApplyPlanID, SessionID: response.SessionID, ArtifactID: response.ArtifactID, StagingPlanID: response.StagingPlanID, TargetRoot: response.TargetRoot, TargetRelativePath: response.TargetRelativePath, SourcePath: response.SourcePath, TargetPath: response.TargetPath, Action: response.Action, Status: response.Status, Issues: response.Issues, CopiedBytes: response.CopiedBytes, VerifiedTargetHash: response.VerifiedTargetHash, ExecutedAt: response.ExecutedAt}, nil
}

func (c *Client) sessionOperation(ctx context.Context, request agent.SessionRequest, operation string) (agent.OperationResult, error) {
	var response SessionOperationResponse
	body := SessionOperationRequest{ProjectID: request.ProjectID, EnvironmentID: request.EnvironmentID, RuntimeProfileID: request.RuntimeProfileID}
	path := "/v1/sessions/" + url.PathEscape(request.SessionID) + "/" + operation
	if err := c.do(ctx, http.MethodPost, path, body, &response); err != nil {
		return agent.OperationResult{}, err
	}
	return agent.OperationResult{AgentID: response.AgentID, Status: response.Status, Message: response.Message, Mode: "http"}, nil
}

func (c *Client) checkpointOperation(ctx context.Context, request agent.CheckpointRequest, operation string) (agent.OperationResult, error) {
	var response CheckpointStubResponse
	body := CheckpointStubRequest{SessionID: request.SessionID, CheckpointID: request.CheckpointID}
	if err := c.do(ctx, http.MethodPost, "/v1/checkpoints/"+operation, body, &response); err != nil {
		return agent.OperationResult{}, err
	}
	return agent.OperationResult{AgentID: response.AgentID, Status: response.Status, Message: response.Message, Mode: "http"}, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, response any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode agent request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	requestURL := *c.baseURL
	requestPath, rawQuery, _ := strings.Cut(path, "?")
	requestURL.Path = strings.TrimRight(c.baseURL.Path, "/") + requestPath
	requestURL.RawQuery = rawQuery
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reader)
	if err != nil {
		return fmt.Errorf("create agent request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	requestID := agent.RequestIDFromContext(ctx)
	if requestID == "" {
		requestID = newRequestID()
	}
	req.Header.Set(requestIDHeader, requestID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call agent %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeHTTPError(resp, path)
	}
	if response == nil {
		return nil
	}
	if err := decodeResponse(resp.Body, response); err != nil {
		return fmt.Errorf("decode agent response for %s: %w", path, err)
	}
	return nil
}

func decodeHTTPError(response *http.Response, operation string) error {
	var payload ErrorResponse
	if err := decodeResponse(response.Body, &payload); err != nil {
		return HTTPError{StatusCode: response.StatusCode, RequestID: response.Header.Get(requestIDHeader), Operation: operation, Message: "malformed error response: " + err.Error()}
	}
	if payload.RequestID == "" {
		payload.RequestID = response.Header.Get(requestIDHeader)
	}
	return HTTPError{StatusCode: response.StatusCode, RequestID: payload.RequestID, AgentID: payload.AgentID, Operation: payload.Operation, Message: payload.Error}
}

func (c *Client) ListAppliedArtifacts(ctx context.Context, sessionID string) (agent.AppliedArtifactsResponse, error) {
	var dto AppliedArtifactsResponse
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+sessionID+"/applied-artifacts", nil, &dto); err != nil {
		return agent.AppliedArtifactsResponse{}, err
	}
	result := agent.AppliedArtifactsResponse{SessionID: dto.SessionID, Records: make([]agent.AppliedArtifactRecord, len(dto.Records))}
	for i, r := range dto.Records {
		result.Records[i] = agent.AppliedArtifactRecord{ApplyPlanID: r.ApplyPlanID, SessionID: r.SessionID, ArtifactID: r.ArtifactID, StagingPlanID: r.StagingPlanID, SourceRuntimeRelativePath: r.SourceRuntimeRelativePath, TargetRuntimeRelativePath: r.TargetRuntimeRelativePath, TargetRoot: r.TargetRoot, TargetRelativePath: r.TargetRelativePath, PayloadAlgorithm: r.PayloadAlgorithm, PayloadHash: r.PayloadHash, PayloadSize: r.PayloadSize, Action: r.Action, Status: r.Status, ActorID: r.ActorID, AppliedAt: r.AppliedAt}
	}
	return result, nil
}

func (c *Client) InspectAppliedArtifact(ctx context.Context, sessionID, applyPlanID string) (agent.AppliedArtifactRecord, error) {
	var dto AppliedArtifactRecordDTO
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+sessionID+"/applied-artifacts/"+applyPlanID, nil, &dto); err != nil {
		return agent.AppliedArtifactRecord{}, err
	}
	return agent.AppliedArtifactRecord{ApplyPlanID: dto.ApplyPlanID, SessionID: dto.SessionID, ArtifactID: dto.ArtifactID, StagingPlanID: dto.StagingPlanID, SourceRuntimeRelativePath: dto.SourceRuntimeRelativePath, TargetRuntimeRelativePath: dto.TargetRuntimeRelativePath, TargetRoot: dto.TargetRoot, TargetRelativePath: dto.TargetRelativePath, PayloadAlgorithm: dto.PayloadAlgorithm, PayloadHash: dto.PayloadHash, PayloadSize: dto.PayloadSize, Action: dto.Action, Status: dto.Status, ActorID: dto.ActorID, AppliedAt: dto.AppliedAt}, nil
}

func (c *Client) VerifyAppliedArtifact(ctx context.Context, sessionID, applyPlanID string) (agent.AppliedArtifactVerification, error) {
	var dto AppliedArtifactVerificationDTO
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+sessionID+"/applied-artifacts/"+applyPlanID+"/verify", nil, &dto); err != nil {
		return agent.AppliedArtifactVerification{}, err
	}
	return agent.AppliedArtifactVerification{SessionID: dto.SessionID, ApplyPlanID: dto.ApplyPlanID, ArtifactID: dto.ArtifactID, StagingPlanID: dto.StagingPlanID, TargetRoot: dto.TargetRoot, TargetRelativePath: dto.TargetRelativePath, TargetRuntimeRelativePath: dto.TargetRuntimeRelativePath, PayloadAlgorithm: dto.PayloadAlgorithm, ExpectedHash: dto.ExpectedHash, ActualHash: dto.ActualHash, PayloadSize: dto.PayloadSize, ActualSize: dto.ActualSize, Status: dto.Status, VerifiedAt: dto.VerifiedAt, ErrorMessage: dto.ErrorMessage}, nil
}

func (c *Client) VerifyAllAppliedArtifacts(ctx context.Context, sessionID string) (agent.BatchAppliedArtifactVerification, error) {
	var dto BatchAppliedArtifactVerificationDTO
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+sessionID+"/applied-artifacts/verify", nil, &dto); err != nil {
		return agent.BatchAppliedArtifactVerification{}, err
	}
	batch := agent.BatchAppliedArtifactVerification{SessionID: dto.SessionID, VerifiedAt: dto.VerifiedAt, Total: dto.Total, ValidCount: dto.ValidCount, MissingCount: dto.MissingCount, CorruptedCount: dto.CorruptedCount, ErrorCount: dto.ErrorCount, Entries: make([]agent.AppliedArtifactVerification, len(dto.Entries))}
	for i, e := range dto.Entries {
		batch.Entries[i] = agent.AppliedArtifactVerification{SessionID: e.SessionID, ApplyPlanID: e.ApplyPlanID, ArtifactID: e.ArtifactID, StagingPlanID: e.StagingPlanID, TargetRoot: e.TargetRoot, TargetRelativePath: e.TargetRelativePath, TargetRuntimeRelativePath: e.TargetRuntimeRelativePath, PayloadAlgorithm: e.PayloadAlgorithm, ExpectedHash: e.ExpectedHash, ActualHash: e.ActualHash, PayloadSize: e.PayloadSize, ActualSize: e.ActualSize, Status: e.Status, VerifiedAt: e.VerifiedAt, ErrorMessage: e.ErrorMessage}
	}
	return batch, nil
}

func decodeResponse(reader io.Reader, target any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("response contains multiple JSON values")
	}
	return nil
}

func (c *Client) MaterializeEnvironment(ctx context.Context, request agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	dto := EnvironmentMaterializationRequest{
		SessionID:              request.SessionID,
		EnvironmentID:          request.EnvironmentID,
		EnvironmentName:        request.EnvironmentName,
		MinecraftVersion:       request.MinecraftVersion,
		JavaVersion:            request.JavaVersion,
		LoaderType:             request.LoaderType,
		LoaderVersion:          request.LoaderVersion,
		ServerCore:             request.ServerCore,
		MCDRRequired:           request.MCDRRequired,
		CarpetRequired:         request.CarpetRequired,
		RuntimeProfileID:       request.RuntimeProfileID,
		RuntimeProfileRequired: request.RuntimeProfileRequired,
		ActorID:                request.ActorID,
	}
	var response EnvironmentMaterializationResponse
	if err := c.do(ctx, http.MethodPost, "/v1/environments/materialize", dto, &response); err != nil {
		return agent.EnvironmentMaterializationResult{}, err
	}
	return agent.EnvironmentMaterializationResult{
		SessionID:              response.SessionID,
		EnvironmentID:          response.EnvironmentID,
		EnvironmentName:        response.EnvironmentName,
		MinecraftVersion:       response.MinecraftVersion,
		JavaVersion:            response.JavaVersion,
		LoaderType:             response.LoaderType,
		LoaderVersion:          response.LoaderVersion,
		ServerCore:             response.ServerCore,
		MCDRRequired:           response.MCDRRequired,
		CarpetRequired:         response.CarpetRequired,
		RuntimeProfileID:       response.RuntimeProfileID,
		RuntimeProfileRequired: response.RuntimeProfileRequired,
		MaterializedAt:         response.MaterializedAt,
		Status:                 response.Status,
		Directories:            response.Directories,
		Metadata:               response.Metadata,
	}, nil
}

func (c *Client) GetSessionRuntimeStatus(ctx context.Context, sessionID string) (agent.SessionRuntimeStatus, error) {
	var response SessionRuntimeStatusResponse
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+sessionID+"/runtime-status", nil, &response); err != nil {
		return agent.SessionRuntimeStatus{}, err
	}
	status := agent.SessionRuntimeStatus{
		SessionID:            response.SessionID,
		CheckedAt:            response.CheckedAt,
		RuntimeRootExists:    response.RuntimeRootExists,
		SessionRootExists:    response.SessionRootExists,
		WorkDirExists:        response.WorkDirExists,
		ConfigDirExists:      response.ConfigDirExists,
		LogsDirExists:        response.LogsDirExists,
		ArtifactsDirExists:   response.ArtifactsDirExists,
		CheckpointsDirExists: response.CheckpointsDirExists,
		TmpDirExists:         response.TmpDirExists,
	}
	if response.EnvironmentManifest != nil {
		status.EnvironmentManifest = &agent.EnvironmentManifestStatus{
			Exists:              response.EnvironmentManifest.Exists,
			Path:                response.EnvironmentManifest.Path,
			RuntimeRelativePath: response.EnvironmentManifest.RuntimeRelativePath,
			Status:              response.EnvironmentManifest.Status,
			EnvironmentID:       response.EnvironmentManifest.EnvironmentID,
			MinecraftVersion:    response.EnvironmentManifest.MinecraftVersion,
			LoaderType:          response.EnvironmentManifest.LoaderType,
			ServerCore:          response.EnvironmentManifest.ServerCore,
			RuntimeProfileID:    response.EnvironmentManifest.RuntimeProfileID,
			MCDRRequired:        response.EnvironmentManifest.MCDRRequired,
			LucyLockHash:        response.EnvironmentManifest.LucyLockHash,
			ErrorMessage:        response.EnvironmentManifest.ErrorMessage,
		}
	}
	if response.MCDRLayout != nil {
		status.MCDRLayout = &agent.MCDRLayoutStatus{
			MCDRRootExists:      response.MCDRLayout.MCDRRootExists,
			ManifestExists:      response.MCDRLayout.ManifestExists,
			ManifestPath:        response.MCDRLayout.ManifestPath,
			RuntimeRelativePath: response.MCDRLayout.RuntimeRelativePath,
		}
	}
	if response.MaterializedArtifacts != nil {
		status.MaterializedArtifacts = &agent.MaterializedArtifactsStatus{
			ManifestExists:      response.MaterializedArtifacts.ManifestExists,
			ManifestPath:        response.MaterializedArtifacts.ManifestPath,
			RuntimeRelativePath: response.MaterializedArtifacts.RuntimeRelativePath,
			Count:               response.MaterializedArtifacts.Count,
		}
	}
	if response.AppliedArtifacts != nil {
		status.AppliedArtifacts = &agent.AppliedArtifactsStatus{
			ManifestExists:      response.AppliedArtifacts.ManifestExists,
			ManifestPath:        response.AppliedArtifacts.ManifestPath,
			RuntimeRelativePath: response.AppliedArtifacts.RuntimeRelativePath,
			Count:               response.AppliedArtifacts.Count,
		}
	}
	if response.ProcessStatus != nil {
		status.ProcessStatus = &agent.ProcessStatusSummary{
			Status:           response.ProcessStatus.Status,
			RuntimeProfileID: response.ProcessStatus.RuntimeProfileID,
			PID:              response.ProcessStatus.PID,
			Crashed:          response.ProcessStatus.Crashed,
			StartedAt:        response.ProcessStatus.StartedAt,
			StoppedAt:        response.ProcessStatus.StoppedAt,
		}
	}
	return status, nil
}

func (c *Client) SessionReadyForStart(ctx context.Context, sessionID string) (agent.SessionStartReadiness, error) {
	var response SessionStartReadinessResponse
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+sessionID+"/ready-for-start", nil, &response); err != nil {
		return agent.SessionStartReadiness{}, err
	}
	issues := make([]agent.SessionStartReadinessIssue, len(response.Issues))
	for index, issue := range response.Issues {
		issues[index] = agent.SessionStartReadinessIssue{Code: issue.Code, Message: issue.Message, Severity: issue.Severity}
	}
	summary := response.RuntimeStatusSummary
	return agent.SessionStartReadiness{
		SessionID: response.SessionID, CheckedAt: response.CheckedAt, Ready: response.Ready, Status: response.Status, Issues: issues,
		RuntimeStatusSummary: agent.SessionStartReadinessSummary{
			RuntimeRootExists: summary.RuntimeRootExists, SessionRootExists: summary.SessionRootExists,
			EnvironmentManifestExists: summary.EnvironmentManifestExists, EnvironmentManifestStatus: summary.EnvironmentManifestStatus,
			WorkDirExists: summary.WorkDirExists, ConfigDirExists: summary.ConfigDirExists, LogsDirExists: summary.LogsDirExists,
			ProcessState: summary.ProcessState, AppliedArtifactsTotal: summary.AppliedArtifactsTotal, AppliedArtifactsValid: summary.AppliedArtifactsValid,
			AppliedArtifactsMissing: summary.AppliedArtifactsMissing, AppliedArtifactsCorrupted: summary.AppliedArtifactsCorrupted, AppliedArtifactsError: summary.AppliedArtifactsError,
		},
	}, nil
}

func (c *Client) InspectMCDRConfigStub(ctx context.Context, sessionID string) (agent.MCDRConfigStubInspection, error) {
	var dto MCDRConfigStubInspectionDTO
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+sessionID+"/mcdr-config-stub", nil, &dto); err != nil {
		return agent.MCDRConfigStubInspection{}, err
	}
	return agent.MCDRConfigStubInspection{
		SessionID:                   dto.SessionID,
		Exists:                      dto.Exists,
		Path:                        dto.Path,
		Valid:                       dto.Valid,
		Status:                      dto.Status,
		PlannedConfigYMLPath:        dto.PlannedConfigYMLPath,
		PlannedServerPropertiesPath: dto.PlannedServerPropertiesPath,
		PlannedEULAPath:             dto.PlannedEULAPath,
		Issues:                      dto.Issues,
		CheckedAt:                   dto.CheckedAt,
	}, nil
}

func (c *Client) SendCommand(ctx context.Context, sessionID, command string) (agent.CommandResult, error) {
	var response SendCommandResponse
	body := SendCommandRequest{Command: command}
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/send-command"
	if err := c.do(ctx, http.MethodPost, path, body, &response); err != nil {
		return agent.CommandResult{}, err
	}
	return agent.CommandResult{AgentID: response.AgentID, Status: response.Status, Message: response.Message}, nil
}

func (c *Client) CreateWorldSnapshot(ctx context.Context, request agent.WorldCheckpointRequest) (agent.WorldCheckpointResult, error) {
	var response WorldCheckpointResponse
	body := WorldCheckpointRequest{WorldDirRel: request.WorldDirRel}
	path := "/v1/sessions/" + url.PathEscape(request.SessionID) + "/world-snapshot"
	if err := c.do(ctx, http.MethodPost, path, body, &response); err != nil {
		return agent.WorldCheckpointResult{}, err
	}
	return agent.WorldCheckpointResult{SessionID: response.SessionID, SnapshotRef: response.SnapshotRef, SizeBytes: response.SizeBytes, SHA256: response.SHA256, CreatedAt: response.CreatedAt}, nil
}

func (c *Client) RestoreWorldSnapshot(ctx context.Context, request agent.WorldCheckpointRestoreRequest) (agent.WorldCheckpointRestoreResult, error) {
	var response WorldCheckpointRestoreResponse
	body := WorldCheckpointRestoreRequest{SnapshotRef: request.SnapshotRef, WorldDirRel: request.WorldDirRel}
	path := "/v1/sessions/" + url.PathEscape(request.SessionID) + "/world-restore"
	if err := c.do(ctx, http.MethodPost, path, body, &response); err != nil {
		return agent.WorldCheckpointRestoreResult{}, err
	}
	return agent.WorldCheckpointRestoreResult{SessionID: response.SessionID, RestoredRef: response.RestoredRef, EntryCount: response.EntryCount, SizeBytes: response.SizeBytes, RestoredAt: response.RestoredAt}, nil
}

func (c *Client) ReadSessionFile(ctx context.Context, sessionID, relativePath string) ([]byte, error) {
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/files/" + relativePath
	fullURL := c.baseURL.String() + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read session file: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
