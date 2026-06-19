package serverjar

import (
	"context"
	"fmt"
	"path/filepath"
)

type DeployRequest struct {
	SessionID        string
	ServerCore       string
	MinecraftVersion string
	LoaderVersion    string
	TargetDir        string
}

type DeployResult struct {
	DeployedPath string
	JarName      string
	SHA256       string
	SizeBytes    int64
	Source       string
}

type Deployer struct {
	downloader *Downloader
}

func NewDeployer(cacheDir string) *Deployer {
	return &Deployer{downloader: NewDownloader(cacheDir)}
}

func (d *Deployer) Deploy(ctx context.Context, req DeployRequest) (DeployResult, error) {
	downloadResult, err := d.downloader.Download(ctx, DownloadRequest{
		ServerCore:       req.ServerCore,
		MinecraftVersion: req.MinecraftVersion,
		LoaderVersion:    req.LoaderVersion,
	})
	if err != nil {
		return DeployResult{}, fmt.Errorf("download server jar: %w", err)
	}
	targetPath := filepath.Join(req.TargetDir, downloadResult.JarName)
	if err := CopyFile(downloadResult.JarPath, targetPath); err != nil {
		return DeployResult{}, fmt.Errorf("deploy server jar: %w", err)
	}
	return DeployResult{
		DeployedPath: targetPath,
		JarName:      downloadResult.JarName,
		SHA256:       downloadResult.SHA256,
		SizeBytes:    downloadResult.SizeBytes,
		Source:       fmt.Sprintf("downloaded-%s", req.ServerCore),
	}, nil
}
