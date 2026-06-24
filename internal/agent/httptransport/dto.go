package httptransport

import (
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

type AgentInfoResponse struct {
	ID              string   `json:"id"`
	Status          string   `json:"status"`
	RuntimeEndpoint string   `json:"runtimeEndpoint"`
	Capabilities    []string `json:"capabilities"`
	RequestID       string   `json:"requestId"`
}

type ResourceReportResponse struct {
	AgentID         string    `json:"agentId"`
	CPUCapacity     int       `json:"cpuCapacity"`
	MemoryTotalMB   int       `json:"memoryTotalMb"`
	MemoryUsedMB    int       `json:"memoryUsedMb"`
	DiskTotalMB     int       `json:"diskTotalMb"`
	DiskUsedMB      int       `json:"diskUsedMb"`
	RunningSessions int       `json:"runningSessions"`
	ReportedAt      time.Time `json:"reportedAt"`
	RequestID       string    `json:"requestId"`
}

type SessionOperationRequest struct {
	ProjectID        string `json:"projectId,omitempty"`
	EnvironmentID    string `json:"environmentId,omitempty"`
	RuntimeProfileID string `json:"runtimeProfileId,omitempty"`
}

type SessionOperationResponse struct {
	AgentID   string `json:"agentId"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

type SessionInspectResponse struct {
	AgentID          string     `json:"agentId"`
	SessionID        string     `json:"sessionId"`
	Status           string     `json:"status"`
	Running          bool       `json:"running"`
	Frozen           bool       `json:"frozen"`
	RuntimeEndpoint  string     `json:"runtimeEndpoint"`
	ProcessID        string     `json:"processId,omitempty"`
	PID              int        `json:"pid,omitempty"`
	RuntimeMode      string     `json:"runtimeMode,omitempty"`
	RuntimeProfileID string     `json:"runtimeProfileId,omitempty"`
	RuntimeType      string     `json:"runtimeType,omitempty"`
	Crashed          bool       `json:"crashed"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	StoppedAt        *time.Time `json:"stoppedAt,omitempty"`
	ExitCode         *int       `json:"exitCode,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
	ObservedAt       time.Time  `json:"observedAt"`
	SessionRoot      string     `json:"sessionRoot,omitempty"`
	WorkDir          string     `json:"workDir,omitempty"`
	LogsDir          string     `json:"logsDir,omitempty"`
	RequestID        string     `json:"requestId"`
}

type RuntimeProfilesResponse struct {
	AgentID   string                   `json:"agentId"`
	Profiles  []runtimeprofile.Profile `json:"profiles"`
	RequestID string                   `json:"requestId"`
}

type LogsResponse struct {
	AgentID   string   `json:"agentId"`
	SessionID string   `json:"sessionId"`
	Lines     []string `json:"lines"`
	RequestID string   `json:"requestId"`
}

type CheckpointStubRequest struct {
	SessionID    string `json:"sessionId"`
	CheckpointID string `json:"checkpointId"`
}

type CheckpointStubResponse = SessionOperationResponse

type ArtifactMaterializationRequest struct {
	SessionID        string `json:"sessionId"`
	ArtifactID       string `json:"artifactId"`
	StagingPlanID    string `json:"stagingPlanId"`
	ArtifactName     string `json:"artifactName"`
	ArtifactType     string `json:"artifactType"`
	TargetName       string `json:"targetName"`
	PayloadAlgorithm string `json:"payloadAlgorithm"`
	PayloadHash      string `json:"payloadHash"`
	PayloadSize      int64  `json:"payloadSize"`
	ActorID          string `json:"actorId"`
	Payload          []byte `json:"payload"`
}

type ArtifactMaterializationResponse struct {
	AgentID             string    `json:"agentId"`
	SessionID           string    `json:"sessionId"`
	ArtifactID          string    `json:"artifactId"`
	StagingPlanID       string    `json:"stagingPlanId"`
	TargetName          string    `json:"targetName"`
	RuntimeRelativePath string    `json:"runtimeRelativePath"`
	PayloadHash         string    `json:"payloadHash"`
	PayloadSize         int64     `json:"payloadSize"`
	MaterializedAt      time.Time `json:"materializedAt"`
	Idempotent          bool      `json:"idempotent"`
	Status              string    `json:"status"`
	RequestID           string    `json:"requestId"`
}

type MaterializedArtifactResponse struct {
	AgentID             string            `json:"agentId,omitempty"`
	SessionID           string            `json:"sessionId,omitempty"`
	ArtifactID          string            `json:"artifactId"`
	StagingPlanID       string            `json:"stagingPlanId,omitempty"`
	ArtifactName        string            `json:"artifactName,omitempty"`
	ArtifactType        string            `json:"artifactType,omitempty"`
	TargetName          string            `json:"targetName"`
	PayloadAlgorithm    string            `json:"payloadAlgorithm,omitempty"`
	PayloadHash         string            `json:"payloadHash"`
	PayloadSize         int64             `json:"payloadSize"`
	RuntimeRelativePath string            `json:"runtimeRelativePath"`
	MaterializedAt      time.Time         `json:"materializedAt,omitempty"`
	ActorID             string            `json:"actorId,omitempty"`
	Status              string            `json:"status,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	RequestID           string            `json:"requestId,omitempty"`
}

type MaterializedArtifactsResponse struct {
	AgentID   string                         `json:"agentId"`
	SessionID string                         `json:"sessionId"`
	Status    string                         `json:"status"`
	Items     []MaterializedArtifactResponse `json:"items"`
	RequestID string                         `json:"requestId"`
}

type MaterializedArtifactVerificationResponse struct {
	AgentID             string    `json:"agentId"`
	SessionID           string    `json:"sessionId"`
	StagingPlanID       string    `json:"stagingPlanId"`
	ArtifactID          string    `json:"artifactId"`
	TargetName          string    `json:"targetName"`
	RuntimeRelativePath string    `json:"runtimeRelativePath"`
	PayloadAlgorithm    string    `json:"payloadAlgorithm"`
	ExpectedHash        string    `json:"expectedHash"`
	ActualHash          string    `json:"actualHash,omitempty"`
	PayloadSize         int64     `json:"payloadSize"`
	ActualSize          int64     `json:"actualSize"`
	Status              string    `json:"status"`
	VerifiedAt          time.Time `json:"verifiedAt"`
	ErrorMessage        string    `json:"errorMessage,omitempty"`
	RequestID           string    `json:"requestId"`
}

type MaterializedArtifactsVerificationResponse struct {
	AgentID        string                                     `json:"agentId"`
	SessionID      string                                     `json:"sessionId"`
	VerifiedAt     time.Time                                  `json:"verifiedAt"`
	Total          int                                        `json:"total"`
	ValidCount     int                                        `json:"validCount"`
	MissingCount   int                                        `json:"missingCount"`
	CorruptedCount int                                        `json:"corruptedCount"`
	ErrorCount     int                                        `json:"errorCount"`
	Entries        []MaterializedArtifactVerificationResponse `json:"entries"`
	RequestID      string                                     `json:"requestId"`
}

type ArtifactApplyDryRunRequestDTO struct {
	ApplyPlanID        string `json:"applyPlanId"`
	SessionID          string `json:"sessionId"`
	StagingPlanID      string `json:"stagingPlanId"`
	ArtifactID         string `json:"artifactId"`
	TargetRoot         string `json:"targetRoot"`
	TargetRelativePath string `json:"targetRelativePath"`
	ExpectedHash       string `json:"expectedHash"`
	ExpectedSize       int64  `json:"expectedSize"`
}

type ArtifactApplyDryRunResultDTO struct {
	AgentID                          string    `json:"agentId"`
	ApplyPlanID                      string    `json:"applyPlanId"`
	SessionID                        string    `json:"sessionId"`
	ArtifactID                       string    `json:"artifactId"`
	StagingPlanID                    string    `json:"stagingPlanId"`
	ApplyKind                        string    `json:"applyKind"`
	TargetRoot                       string    `json:"targetRoot"`
	TargetRelativePath               string    `json:"targetRelativePath"`
	SourceRuntimeRelativePath        string    `json:"sourceRuntimeRelativePath"`
	PlannedTargetRuntimeRelativePath string    `json:"plannedTargetRuntimeRelativePath"`
	Action                           string    `json:"action"`
	Status                           string    `json:"status"`
	Issues                           []string  `json:"issues"`
	CheckedAt                        time.Time `json:"checkedAt"`
	RequestID                        string    `json:"requestId"`
}

type ArtifactApplyExecuteRequestDTO struct {
	ApplyPlanID        string `json:"applyPlanId"`
	SessionID          string `json:"sessionId"`
	StagingPlanID      string `json:"stagingPlanId"`
	ArtifactID         string `json:"artifactId"`
	TargetRoot         string `json:"targetRoot"`
	TargetRelativePath string `json:"targetRelativePath"`
	ExpectedHash       string `json:"expectedHash"`
	ExpectedSize       int64  `json:"expectedSize"`
}

type ArtifactApplyExecuteResultDTO struct {
	AgentID            string    `json:"agentId"`
	ApplyPlanID        string    `json:"applyPlanId"`
	SessionID          string    `json:"sessionId"`
	ArtifactID         string    `json:"artifactId"`
	StagingPlanID      string    `json:"stagingPlanId"`
	TargetRoot         string    `json:"targetRoot"`
	TargetRelativePath string    `json:"targetRelativePath"`
	SourcePath         string    `json:"sourcePath"`
	TargetPath         string    `json:"targetPath"`
	Action             string    `json:"action"`
	Status             string    `json:"status"`
	Issues             []string  `json:"issues"`
	CopiedBytes        int64     `json:"copiedBytes"`
	VerifiedTargetHash string    `json:"verifiedTargetHash"`
	ExecutedAt         time.Time `json:"executedAt"`
	RequestID          string    `json:"requestId"`
}

type AppliedArtifactRecordDTO struct {
	ApplyPlanID               string    `json:"applyPlanId"`
	SessionID                 string    `json:"sessionId"`
	ArtifactID                string    `json:"artifactId"`
	StagingPlanID             string    `json:"stagingPlanId"`
	SourceRuntimeRelativePath string    `json:"sourceRuntimeRelativePath"`
	TargetRuntimeRelativePath string    `json:"targetRuntimeRelativePath"`
	TargetRoot                string    `json:"targetRoot"`
	TargetRelativePath        string    `json:"targetRelativePath"`
	PayloadAlgorithm          string    `json:"payloadAlgorithm"`
	PayloadHash               string    `json:"payloadHash"`
	PayloadSize               int64     `json:"payloadSize"`
	Action                    string    `json:"action"`
	Status                    string    `json:"status"`
	ActorID                   string    `json:"actorId,omitempty"`
	AppliedAt                 time.Time `json:"appliedAt"`
}

type AppliedArtifactsResponse struct {
	SessionID string                     `json:"sessionId"`
	Records   []AppliedArtifactRecordDTO `json:"records"`
	RequestID string                     `json:"requestId"`
}

type AppliedArtifactVerificationDTO struct {
	SessionID                 string    `json:"sessionId"`
	ApplyPlanID               string    `json:"applyPlanId"`
	ArtifactID                string    `json:"artifactId"`
	StagingPlanID             string    `json:"stagingPlanId"`
	TargetRoot                string    `json:"targetRoot"`
	TargetRelativePath        string    `json:"targetRelativePath"`
	TargetRuntimeRelativePath string    `json:"targetRuntimeRelativePath"`
	PayloadAlgorithm          string    `json:"payloadAlgorithm"`
	ExpectedHash              string    `json:"expectedHash"`
	ActualHash                string    `json:"actualHash,omitempty"`
	PayloadSize               int64     `json:"payloadSize"`
	ActualSize                int64     `json:"actualSize"`
	Status                    string    `json:"status"`
	VerifiedAt                time.Time `json:"verifiedAt"`
	ErrorMessage              string    `json:"errorMessage,omitempty"`
	RequestID                 string    `json:"requestId"`
}

type BatchAppliedArtifactVerificationDTO struct {
	SessionID      string                           `json:"sessionId"`
	VerifiedAt     time.Time                        `json:"verifiedAt"`
	Total          int                              `json:"total"`
	ValidCount     int                              `json:"validCount"`
	MissingCount   int                              `json:"missingCount"`
	CorruptedCount int                              `json:"corruptedCount"`
	ErrorCount     int                              `json:"errorCount"`
	Entries        []AppliedArtifactVerificationDTO `json:"entries"`
	RequestID      string                           `json:"requestId"`
}

type ErrorResponse struct {
	Error     string `json:"error"`
	Operation string `json:"operation,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
}

type EnvironmentMaterializationRequest struct {
	SessionID              string                `json:"sessionId"`
	EnvironmentID          string                `json:"environmentId"`
	EnvironmentName        string                `json:"environmentName"`
	MinecraftVersion       string                `json:"minecraftVersion"`
	JavaVersion            string                `json:"javaVersion"`
	LoaderType             string                `json:"loaderType"`
	LoaderVersion          string                `json:"loaderVersion"`
	ServerCore             string                `json:"serverCore"`
	MCDRRequired           bool                  `json:"mcdrRequired"`
	CarpetRequired         bool                  `json:"carpetRequired"`
	LucyManifestRef        string                `json:"lucyManifestRef,omitempty"`
	LucyLockRef            string                `json:"lucyLockRef,omitempty"`
	Packages               []PackageRefDTO       `json:"packages,omitempty"`
	LocalArtifacts         []LocalArtifactRefDTO `json:"localArtifacts,omitempty"`
	RuntimeProfileID       string                `json:"runtimeProfileId"`
	RuntimeProfileRequired bool                  `json:"runtimeProfileRequired"`
	ActorID                string                `json:"actorId"`
}

type PackageRefDTO struct {
	ID                string            `json:"id"`
	Source            string            `json:"source"`
	Name              string            `json:"name"`
	VersionConstraint string            `json:"versionConstraint,omitempty"`
	MinecraftVersion  string            `json:"minecraftVersion,omitempty"`
	Loader            string            `json:"loader,omitempty"`
	Required          bool              `json:"required"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type LocalArtifactRefDTO struct {
	ArtifactID       string            `json:"artifactId"`
	PayloadAlgorithm string            `json:"payloadAlgorithm"`
	PayloadHash      string            `json:"payloadHash"`
	PayloadSize      int64             `json:"payloadSize"`
	ArtifactType     string            `json:"artifactType"`
	RuntimeName      string            `json:"runtimeName"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type EnvironmentMaterializationResponse struct {
	SessionID              string            `json:"sessionId"`
	EnvironmentID          string            `json:"environmentId"`
	EnvironmentName        string            `json:"environmentName"`
	MinecraftVersion       string            `json:"minecraftVersion"`
	JavaVersion            string            `json:"javaVersion"`
	LoaderType             string            `json:"loaderType"`
	LoaderVersion          string            `json:"loaderVersion"`
	ServerCore             string            `json:"serverCore"`
	MCDRRequired           bool              `json:"mcdrRequired"`
	CarpetRequired         bool              `json:"carpetRequired"`
	RuntimeProfileID       string            `json:"runtimeProfileId"`
	RuntimeProfileRequired bool              `json:"runtimeProfileRequired"`
	MaterializedAt         time.Time         `json:"materializedAt"`
	Status                 string            `json:"status"`
	Directories            []string          `json:"directories"`
	Metadata               map[string]string `json:"metadata"`
	RequestID              string            `json:"requestId"`
}

type SessionRuntimeStatusResponse struct {
	SessionID             string                          `json:"sessionId"`
	CheckedAt             time.Time                       `json:"checkedAt"`
	RuntimeRootExists     bool                            `json:"runtimeRootExists"`
	SessionRootExists     bool                            `json:"sessionRootExists"`
	WorkDirExists         bool                            `json:"workDirExists"`
	ConfigDirExists       bool                            `json:"configDirExists"`
	LogsDirExists         bool                            `json:"logsDirExists"`
	ArtifactsDirExists    bool                            `json:"artifactsDirExists"`
	CheckpointsDirExists  bool                            `json:"checkpointsDirExists"`
	TmpDirExists          bool                            `json:"tmpDirExists"`
	EnvironmentManifest   *EnvironmentManifestStatusDTO   `json:"environmentManifest,omitempty"`
	MCDRLayout            *MCDRLayoutStatusDTO            `json:"mcdrLayout,omitempty"`
	MaterializedArtifacts *MaterializedArtifactsStatusDTO `json:"materializedArtifacts,omitempty"`
	AppliedArtifacts      *AppliedArtifactsStatusDTO      `json:"appliedArtifacts,omitempty"`
	ProcessStatus         *ProcessStatusSummaryDTO        `json:"processStatus,omitempty"`
	WorldProfile          *WorldProfileStatusDTO          `json:"worldProfile,omitempty"`
	RequestID             string                          `json:"requestId"`
}

type WorldProfileStatusDTO struct {
	Seed               string `json:"seed,omitempty"`
	LevelType          string `json:"levelType"`
	GeneratorSettings  string `json:"generatorSettings,omitempty"`
	GenerateStructures bool   `json:"generateStructures"`
	SpawnRadius        int    `json:"spawnRadius"`
	Difficulty         string `json:"difficulty"`
	ViewDistance       int    `json:"viewDistance,omitempty"`
}

type EnvironmentManifestStatusDTO struct {
	Exists              bool   `json:"exists"`
	Path                string `json:"path,omitempty"`
	RuntimeRelativePath string `json:"runtimeRelativePath,omitempty"`
	Status              string `json:"status,omitempty"`
	EnvironmentID       string `json:"environmentId,omitempty"`
	MinecraftVersion    string `json:"minecraftVersion,omitempty"`
	LoaderType          string `json:"loaderType,omitempty"`
	ServerCore          string `json:"serverCore,omitempty"`
	RuntimeProfileID    string `json:"runtimeProfileId,omitempty"`
	MCDRRequired        bool   `json:"mcdrRequired"`
	LucyLockHash        string `json:"lucyLockHash,omitempty"`
	ErrorMessage        string `json:"errorMessage,omitempty"`
}

type SessionStartReadinessResponse struct {
	SessionID            string                       `json:"sessionId"`
	CheckedAt            time.Time                    `json:"checkedAt"`
	Ready                bool                         `json:"ready"`
	Status               string                       `json:"status"`
	Issues               []SessionStartReadinessIssue `json:"issues"`
	RuntimeStatusSummary SessionStartReadinessSummary `json:"runtimeStatusSummary"`
	RequestID            string                       `json:"requestId"`
}

type SessionStartReadinessIssue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type SessionStartReadinessSummary struct {
	RuntimeRootExists         bool   `json:"runtimeRootExists"`
	SessionRootExists         bool   `json:"sessionRootExists"`
	EnvironmentManifestExists bool   `json:"environmentManifestExists"`
	EnvironmentManifestStatus string `json:"environmentManifestStatus,omitempty"`
	WorkDirExists             bool   `json:"workDirExists"`
	ConfigDirExists           bool   `json:"configDirExists"`
	LogsDirExists             bool   `json:"logsDirExists"`
	ProcessState              string `json:"processState"`
	AppliedArtifactsTotal     int    `json:"appliedArtifactsTotal"`
	AppliedArtifactsValid     int    `json:"appliedArtifactsValid"`
	AppliedArtifactsMissing   int    `json:"appliedArtifactsMissing"`
	AppliedArtifactsCorrupted int    `json:"appliedArtifactsCorrupted"`
	AppliedArtifactsError     int    `json:"appliedArtifactsError"`
}

type MCDRLayoutStatusDTO struct {
	MCDRRootExists      bool   `json:"mcdrRootExists"`
	ManifestExists      bool   `json:"manifestExists"`
	ManifestPath        string `json:"manifestPath,omitempty"`
	RuntimeRelativePath string `json:"runtimeRelativePath,omitempty"`
}

type MaterializedArtifactsStatusDTO struct {
	ManifestExists      bool   `json:"manifestExists"`
	ManifestPath        string `json:"manifestPath,omitempty"`
	RuntimeRelativePath string `json:"runtimeRelativePath,omitempty"`
	Count               int    `json:"count"`
}

type AppliedArtifactsStatusDTO struct {
	ManifestExists      bool   `json:"manifestExists"`
	ManifestPath        string `json:"manifestPath,omitempty"`
	RuntimeRelativePath string `json:"runtimeRelativePath,omitempty"`
	Count               int    `json:"count"`
}

type ProcessStatusSummaryDTO struct {
	Status           string     `json:"status"`
	RuntimeProfileID string     `json:"runtimeProfileId,omitempty"`
	PID              int        `json:"pid,omitempty"`
	Crashed          bool       `json:"crashed"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	StoppedAt        *time.Time `json:"stoppedAt,omitempty"`
}

func sessionRuntimeStatusResponse(status agent.SessionRuntimeStatus, requestID string) SessionRuntimeStatusResponse {
	response := SessionRuntimeStatusResponse{
		SessionID:            status.SessionID,
		CheckedAt:            status.CheckedAt,
		RuntimeRootExists:    status.RuntimeRootExists,
		SessionRootExists:    status.SessionRootExists,
		WorkDirExists:        status.WorkDirExists,
		ConfigDirExists:      status.ConfigDirExists,
		LogsDirExists:        status.LogsDirExists,
		ArtifactsDirExists:   status.ArtifactsDirExists,
		CheckpointsDirExists: status.CheckpointsDirExists,
		TmpDirExists:         status.TmpDirExists,
		RequestID:            requestID,
	}
	if status.EnvironmentManifest != nil {
		response.EnvironmentManifest = &EnvironmentManifestStatusDTO{
			Exists:              status.EnvironmentManifest.Exists,
			Path:                status.EnvironmentManifest.Path,
			RuntimeRelativePath: status.EnvironmentManifest.RuntimeRelativePath,
			Status:              status.EnvironmentManifest.Status,
			EnvironmentID:       status.EnvironmentManifest.EnvironmentID,
			MinecraftVersion:    status.EnvironmentManifest.MinecraftVersion,
			LoaderType:          status.EnvironmentManifest.LoaderType,
			ServerCore:          status.EnvironmentManifest.ServerCore,
			RuntimeProfileID:    status.EnvironmentManifest.RuntimeProfileID,
			MCDRRequired:        status.EnvironmentManifest.MCDRRequired,
			LucyLockHash:        status.EnvironmentManifest.LucyLockHash,
			ErrorMessage:        status.EnvironmentManifest.ErrorMessage,
		}
	}
	if status.MCDRLayout != nil {
		response.MCDRLayout = &MCDRLayoutStatusDTO{
			MCDRRootExists:      status.MCDRLayout.MCDRRootExists,
			ManifestExists:      status.MCDRLayout.ManifestExists,
			ManifestPath:        status.MCDRLayout.ManifestPath,
			RuntimeRelativePath: status.MCDRLayout.RuntimeRelativePath,
		}
	}
	if status.MaterializedArtifacts != nil {
		response.MaterializedArtifacts = &MaterializedArtifactsStatusDTO{
			ManifestExists:      status.MaterializedArtifacts.ManifestExists,
			ManifestPath:        status.MaterializedArtifacts.ManifestPath,
			RuntimeRelativePath: status.MaterializedArtifacts.RuntimeRelativePath,
			Count:               status.MaterializedArtifacts.Count,
		}
	}
	if status.AppliedArtifacts != nil {
		response.AppliedArtifacts = &AppliedArtifactsStatusDTO{
			ManifestExists:      status.AppliedArtifacts.ManifestExists,
			ManifestPath:        status.AppliedArtifacts.ManifestPath,
			RuntimeRelativePath: status.AppliedArtifacts.RuntimeRelativePath,
			Count:               status.AppliedArtifacts.Count,
		}
	}
	if status.ProcessStatus != nil {
		response.ProcessStatus = &ProcessStatusSummaryDTO{
			Status:           status.ProcessStatus.Status,
			RuntimeProfileID: status.ProcessStatus.RuntimeProfileID,
			PID:              status.ProcessStatus.PID,
			Crashed:          status.ProcessStatus.Crashed,
			StartedAt:        status.ProcessStatus.StartedAt,
			StoppedAt:        status.ProcessStatus.StoppedAt,
		}
	}
	if status.WorldProfile != nil {
		response.WorldProfile = &WorldProfileStatusDTO{
			Seed:               status.WorldProfile.Seed,
			LevelType:          status.WorldProfile.LevelType,
			GeneratorSettings:  status.WorldProfile.GeneratorSettings,
			GenerateStructures: status.WorldProfile.GenerateStructures,
			SpawnRadius:        status.WorldProfile.SpawnRadius,
			Difficulty:         status.WorldProfile.Difficulty,
			ViewDistance:       status.WorldProfile.ViewDistance,
		}
	}
	return response
}

type MCDRConfigStubInspectionDTO struct {
	SessionID                   string    `json:"sessionId"`
	Exists                      bool      `json:"exists"`
	Path                        string    `json:"path"`
	Valid                       bool      `json:"valid"`
	Status                      string    `json:"status"`
	PlannedConfigYMLPath        string    `json:"plannedConfigYmlPath,omitempty"`
	PlannedServerPropertiesPath string    `json:"plannedServerPropertiesPath,omitempty"`
	PlannedEULAPath             string    `json:"plannedEulaPath,omitempty"`
	Issues                      []string  `json:"issues,omitempty"`
	CheckedAt                   time.Time `json:"checkedAt"`
}

type SendCommandRequest struct {
	Command string `json:"command"`
}

type SendCommandResponse struct {
	AgentID   string `json:"agentId"`
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

type WorldCheckpointRequest struct {
	WorldDirRel string `json:"worldDirRel"`
}

type WorldCheckpointResponse struct {
	SessionID   string    `json:"sessionId"`
	SnapshotRef string    `json:"snapshotRef"`
	SizeBytes   int64     `json:"sizeBytes"`
	SHA256      string    `json:"sha256"`
	CreatedAt   time.Time `json:"createdAt"`
	RequestID   string    `json:"requestId"`
}

type WorldCheckpointRestoreRequest struct {
	SnapshotRef string `json:"snapshotRef"`
	WorldDirRel string `json:"worldDirRel"`
}

type WorldCheckpointRestoreResponse struct {
	SessionID   string    `json:"sessionId"`
	RestoredRef string    `json:"restoredRef"`
	EntryCount  int       `json:"entryCount"`
	SizeBytes   int64     `json:"sizeBytes"`
	RestoredAt  time.Time `json:"restoredAt"`
	RequestID   string    `json:"requestId"`
}
