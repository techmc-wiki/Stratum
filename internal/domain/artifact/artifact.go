package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
)

type Type string

const (
	TypeJar          Type = "jar"
	TypeDatapack     Type = "datapack"
	TypeMCDRPlugin   Type = "mcdr-plugin"
	TypeConfigPreset Type = "config-preset"
	TypeCarpetRules  Type = "carpet-rules"
	TypeSchematic    Type = "schematic"
	TypeWorldArchive Type = "world-archive"
)

type Status string
type PayloadStatus string

const (
	StatusPending    Status = "pending"
	StatusApproved   Status = "approved"
	StatusRejected   Status = "rejected"
	StatusDeprecated Status = "deprecated"
)

const (
	PayloadMetadataOnly PayloadStatus = "metadata-only"
	PayloadAvailable    PayloadStatus = "available"
)

type Usage struct {
	SessionID string    `json:"sessionId"`
	UsedAt    time.Time `json:"usedAt"`
}

type Artifact struct {
	ID                      string        `json:"id"`
	ProjectID               string        `json:"projectId,omitempty"`
	Name                    string        `json:"name"`
	Type                    Type          `json:"type"`
	UploaderID              string        `json:"uploaderId"`
	SHA256                  string        `json:"sha256"`
	SizeBytes               int64         `json:"sizeBytes"`
	PayloadStatus           PayloadStatus `json:"payloadStatus,omitempty"`
	TargetMinecraftVersions []string      `json:"targetMinecraftVersions"`
	LoaderCompatibility     []string      `json:"loaderCompatibility"`
	Status                  Status        `json:"status"`
	UsageRecords            []Usage       `json:"usageRecords,omitempty"`
	ReviewNotes             string        `json:"reviewNotes,omitempty"`
	ReviewedBy              string        `json:"reviewedBy,omitempty"`
	ReviewedAt              *time.Time    `json:"reviewedAt,omitempty"`
	ReviewReason            string        `json:"reviewReason,omitempty"`
	CreatedAt               time.Time     `json:"createdAt"`
}

func ValidateType(value Type) error {
	switch value {
	case TypeJar, TypeDatapack, TypeMCDRPlugin, TypeConfigPreset, TypeCarpetRules, TypeSchematic, TypeWorldArchive:
		return nil
	default:
		return fmt.Errorf("unsupported artifact type %q", value)
	}
}

func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func HashFile(path string) (hash string, size int64, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	digest := sha256.New()
	size, err = io.Copy(digest, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}
