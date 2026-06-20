package serverjar

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadFabric(t *testing.T) {
	if err := SetProxy(os.Getenv("STRATUM_PROXY")); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(t.TempDir(), "cache")
	downloader := NewDownloader(cacheDir)

	result, err := downloader.Download(context.Background(), DownloadRequest{
		ServerCore:       "fabric",
		MinecraftVersion: "1.17.1",
		LoaderVersion:    "0.11.7",
	})
	if err != nil {
		t.Fatalf("download Fabric: %v", err)
	}
	if !strings.Contains(result.JarName, "fabric-server") {
		t.Fatalf("unexpected jar name: %s", result.JarName)
	}
	if result.SHA256 == "" || result.SizeBytes == 0 {
		t.Fatalf("missing hash/size: %+v", result)
	}
	if _, err := os.Stat(result.JarPath); err != nil {
		t.Fatalf("downloaded jar not found: %v", err)
	}
}

func TestDownloadVanilla(t *testing.T) {
	if err := SetProxy(os.Getenv("STRATUM_PROXY")); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(t.TempDir(), "cache")
	downloader := NewDownloader(cacheDir)

	result, err := downloader.Download(context.Background(), DownloadRequest{
		ServerCore:       "minecraft",
		MinecraftVersion: "1.17.1",
	})
	if err != nil {
		t.Fatalf("download Vanilla: %v", err)
	}
	if result.SHA256 == "" || result.SizeBytes == 0 {
		t.Fatalf("missing hash/size: %+v", result)
	}
	if _, err := os.Stat(result.JarPath); err != nil {
		t.Fatalf("downloaded jar not found: %v", err)
	}
}

func TestDownloadPaper(t *testing.T) {
	if err := SetProxy(os.Getenv("STRATUM_PROXY")); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(t.TempDir(), "cache")
	downloader := NewDownloader(cacheDir)

	result, err := downloader.Download(context.Background(), DownloadRequest{
		ServerCore:       "paper",
		MinecraftVersion: "1.17.1",
	})
	if err != nil {
		t.Fatalf("download Paper: %v", err)
	}
	if !strings.Contains(result.JarName, "paper") {
		t.Fatalf("unexpected jar name: %s", result.JarName)
	}
	if result.SHA256 == "" || result.SizeBytes == 0 {
		t.Fatalf("missing hash/size: %+v", result)
	}
	if _, err := os.Stat(result.JarPath); err != nil {
		t.Fatalf("downloaded jar not found: %v", err)
	}
}

func TestDownloadUnsupportedCore(t *testing.T) {
	cacheDir := t.TempDir()
	downloader := NewDownloader(cacheDir)
	_, err := downloader.Download(context.Background(), DownloadRequest{
		ServerCore: "neoforge",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported server core") {
		t.Fatalf("expected unsupported core error: %v", err)
	}
}

func TestDownloadForgeSupported(t *testing.T) {
	cacheDir := t.TempDir()
	downloader := NewDownloader(cacheDir)
	_, err := downloader.Download(context.Background(), DownloadRequest{
		ServerCore:       "forge",
		MinecraftVersion: "1.12.2",
		LoaderVersion:    "14.23.5.2859",
	})
	if err != nil {
		t.Fatalf("download Forge 1.12.2: %v", err)
	}
}

func TestResolveLatestVersion(t *testing.T) {
	version, err := ResolveLatestVersion(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatestVersion: %v", err)
	}
	if version == "" {
		t.Fatal("empty latest version")
	}
	t.Logf("Latest Minecraft version: %s", version)
}

func TestDownloadLatestVanilla(t *testing.T) {
	cacheDir := t.TempDir()
	downloader := NewDownloader(cacheDir)
	result, err := downloader.Download(context.Background(), DownloadRequest{
		ServerCore:       "vanilla",
		MinecraftVersion: "latest",
	})
	if err != nil {
		t.Fatalf("download latest vanilla: %v", err)
	}
	if result.JarName == "" || result.SHA256 == "" {
		t.Fatalf("incomplete result: %+v", result)
	}
	t.Logf("Latest vanilla: %s (%d bytes, sha256=%s)", result.JarName, result.SizeBytes, result.SHA256[:16])
}

func TestDeployServers(t *testing.T) {
	if err := SetProxy(os.Getenv("STRATUM_PROXY")); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	targetDir := filepath.Join(root, "server")
	deployer := NewDeployer(cacheDir)

	result, err := deployer.Deploy(context.Background(), DeployRequest{
		SessionID:        "test-fabric",
		ServerCore:       "fabric",
		MinecraftVersion: "1.17.1",
		LoaderVersion:    "0.11.7",
		TargetDir:        targetDir,
	})
	if err != nil {
		t.Fatalf("deploy Fabric: %v", err)
	}
	if _, err := os.Stat(result.DeployedPath); err != nil {
		t.Fatalf("deployed jar not found: %v", err)
	}
	if result.SHA256 == "" || result.SizeBytes == 0 {
		t.Fatalf("missing hash/size in result: %+v", result)
	}
}

func TestDeployFabricLatestLoader(t *testing.T) {
	if err := SetProxy(os.Getenv("STRATUM_PROXY")); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	targetDir := filepath.Join(root, "server")
	deployer := NewDeployer(cacheDir)

	result, err := deployer.Deploy(context.Background(), DeployRequest{
		SessionID:        "test-fabric-latest",
		ServerCore:       "fabric",
		MinecraftVersion: "1.21.1",
		LoaderVersion:    "latest",
		TargetDir:        targetDir,
	})
	if err != nil {
		t.Fatalf("deploy Fabric latest: %v", err)
	}
	if _, err := os.Stat(result.DeployedPath); err != nil {
		t.Fatalf("deployed jar not found: %v", err)
	}
}
