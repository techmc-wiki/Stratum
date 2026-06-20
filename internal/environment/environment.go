package environment

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type LoaderType string

const (
	LoaderNone       LoaderType = "none"
	LoaderFabric     LoaderType = "fabric"
	LoaderLiteLoader LoaderType = "liteloader"
	LoaderForge      LoaderType = "forge"
	LoaderQuilt      LoaderType = "quilt"
	LoaderCustom     LoaderType = "custom"
)

type ServerCore string

const (
	ServerVanilla ServerCore = "vanilla"
	ServerFabric  ServerCore = "fabric"
	ServerCarpet  ServerCore = "carpet"
	ServerPaper   ServerCore = "paper"
	ServerForge   ServerCore = "forge"
	ServerCustom  ServerCore = "custom"
)

type Environment struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	Description            string            `json:"description"`
	MinecraftVersion       string            `json:"minecraftVersion"`
	JavaVersion            string            `json:"javaVersion"`
	LoaderType             LoaderType        `json:"loaderType"`
	LoaderVersion          string            `json:"loaderVersion"`
	ServerCore             ServerCore        `json:"serverCore"`
	MCDRRequired           bool              `json:"mcdrRequired"`
	CarpetRequired         bool              `json:"carpetRequired"`
	LucyManifestRef        string            `json:"lucyManifestRef"`
	LucyLockRef            string            `json:"lucyLockRef"`
	RuntimeProfileID       string            `json:"runtimeProfileId"`
	RuntimeProfileRequired bool              `json:"runtimeProfileRequired"`
	ArtifactPolicyID       string            `json:"artifactPolicyId"`
	PythonManagerType      string            `json:"pythonManagerType"`
	Notes                  string            `json:"notes"`
	CreatedAt              time.Time         `json:"createdAt"`
	UpdatedAt              time.Time         `json:"updatedAt"`
	Metadata               map[string]string `json:"metadata"`
}

func (e Environment) Validate() error {
	if e.ID == "" || e.ID == "." || e.ID == ".." {
		return errors.New("environment id is required")
	}
	if strings.Contains(e.ID, "/") || strings.Contains(e.ID, "\\") {
		return fmt.Errorf("environment id contains unsafe characters: %q", e.ID)
	}
	if e.Name == "" {
		return errors.New("environment name is required")
	}
	if e.MinecraftVersion == "" {
		return errors.New("minecraft version is required")
	}
	if e.JavaVersion != "" && len(e.JavaVersion) == 0 {
		return errors.New("java version must be non-empty if provided")
	}
	if !isValidLoaderType(e.LoaderType) {
		return fmt.Errorf("invalid loader type: %q", e.LoaderType)
	}
	if !isValidServerCore(e.ServerCore) {
		return fmt.Errorf("invalid server core: %q", e.ServerCore)
	}
	if e.RuntimeProfileID != "" && (strings.Contains(e.RuntimeProfileID, "/") || strings.Contains(e.RuntimeProfileID, "\\")) {
		return fmt.Errorf("runtime profile id contains unsafe characters: %q", e.RuntimeProfileID)
	}
	return nil
}

func isValidLoaderType(t LoaderType) bool {
	switch t {
	case LoaderNone, LoaderFabric, LoaderLiteLoader, LoaderForge, LoaderQuilt, LoaderCustom:
		return true
	}
	return false
}

func isValidServerCore(c ServerCore) bool {
	switch c {
	case ServerVanilla, ServerFabric, ServerCarpet, ServerPaper, ServerForge, ServerCustom:
		return true
	}
	return false
}
