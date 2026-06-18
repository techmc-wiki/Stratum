package agent

import (
	"fmt"
	"time"

	"github.com/stratummc/stratum/internal/integration/lucy"
)

type EnvironmentMaterializationRequest struct {
	SessionID              string
	EnvironmentID          string
	EnvironmentName        string
	MinecraftVersion       string
	JavaVersion            string
	LoaderType             string
	LoaderVersion          string
	ServerCore             string
	MCDRRequired           bool
	CarpetRequired         bool
	LucyManifestRef        string
	LucyLockRef            string
	Packages               []lucy.PackageRef
	LocalArtifacts         []lucy.LocalArtifactRef
	RuntimeProfileID       string
	RuntimeProfileRequired bool
	ActorID                string
}

type EnvironmentMaterializationResult struct {
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
	LucyResolutionStatus   string            `json:"lucyResolutionStatus"`
	LucyLockHash           string            `json:"lucyLockHash,omitempty"`
	LucyManifestPath       string            `json:"lucyManifestPath,omitempty"`
	LucyLockPath           string            `json:"lucyLockPath,omitempty"`
	LucyInstallStatus      string            `json:"lucyInstallStatus,omitempty"`
	LucyInstalledCount     int               `json:"lucyInstalledCount,omitempty"`
	LucyFailedCount        int               `json:"lucyFailedCount,omitempty"`
	LucyInstallTotalSize   int64             `json:"lucyInstallTotalSize,omitempty"`
	LucyIntegrityStatus    string            `json:"lucyIntegrityStatus,omitempty"`
	MaterializedAt         time.Time         `json:"materializedAt"`
	Status                 string            `json:"status"`
	Directories            []string          `json:"directories"`
	Metadata               map[string]string `json:"metadata"`
}

type EnvironmentIntegrityError struct {
	SessionID string
	Status    string
	Missing   []string
	Corrupt   []string
	Errors    []string
}

func NewEnvironmentIntegrityError(sessionID, status string, missing, corrupt, errs []string) *EnvironmentIntegrityError {
	return &EnvironmentIntegrityError{SessionID: sessionID, Status: status, Missing: missing, Corrupt: corrupt, Errors: errs}
}

func (e *EnvironmentIntegrityError) Error() string {
	return fmt.Sprintf("environment integrity check failed for session %s: status=%s missing=%d corrupt=%d errors=%d", e.SessionID, e.Status, len(e.Missing), len(e.Corrupt), len(e.Errors))
}
