package agent

import (
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
	MaterializedAt         time.Time         `json:"materializedAt"`
	Status                 string            `json:"status"`
	Directories            []string          `json:"directories"`
	Metadata               map[string]string `json:"metadata"`
}
