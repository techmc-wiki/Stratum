package lucy

import (
	"context"
	"fmt"

	lucyartifact "github.com/mclucy/lucy/artifact"
)

// ArtifactService provides operations for Lucy artifact analysis.
type ArtifactService struct{}

// NewArtifactService creates a new ArtifactService.
func NewArtifactService() *ArtifactService {
	return &ArtifactService{}
}

// ArtifactInfo wraps Lucy's artifact metadata.
type ArtifactInfo struct {
	Platform string
	Name     string
	Version  string
}

// Analyze extracts metadata from a JAR, ZIP, or MCDR plugin file.
func (s *ArtifactService) Analyze(ctx context.Context, filePath string) ([]ArtifactInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	infos, err := lucyartifact.Analyze(filePath)
	if err != nil {
		return nil, fmt.Errorf("analyze artifact: %w", err)
	}
	result := make([]ArtifactInfo, len(infos))
	for i, info := range infos {
		result[i] = ArtifactInfo{
			Platform: string(info.Ref.Eco),
			Name:     string(info.Ref.Name),
			Version:  string(info.Version),
		}
	}
	return result, nil
}
