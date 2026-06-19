package serverjar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 120 * time.Second}

func SetProxy(proxyURL string) error {
	if strings.TrimSpace(proxyURL) == "" {
		httpClient.Transport = nil
		return nil
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("parse proxy URL: %w", err)
	}
	httpClient.Transport = &http.Transport{Proxy: http.ProxyURL(parsed)}
	return nil
}

type DownloadRequest struct {
	ServerCore       string
	MinecraftVersion string
	LoaderVersion    string
}

type DownloadResult struct {
	JarPath   string
	JarName   string
	SHA256    string
	SizeBytes int64
	Version   string
}

type Downloader struct {
	CacheDir string
}

func NewDownloader(cacheDir string) *Downloader {
	return &Downloader{CacheDir: cacheDir}
}

func (d *Downloader) Download(ctx context.Context, req DownloadRequest) (DownloadResult, error) {
	switch strings.ToLower(req.ServerCore) {
	case "minecraft", "vanilla":
		return d.downloadVanilla(ctx, req.MinecraftVersion)
	case "fabric":
		return d.downloadFabric(ctx, req.MinecraftVersion, req.LoaderVersion)
	case "paper":
		return d.downloadPaper(ctx, req.MinecraftVersion)
	default:
		return DownloadResult{}, fmt.Errorf("unsupported server core for download: %s (use custom for uploaded jars)", req.ServerCore)
	}
}

func (d *Downloader) downloadVanilla(ctx context.Context, mcVersion string) (DownloadResult, error) {
	manifest, err := fetchJSON(ctx, "https://launchermeta.mojang.com/mc/game/version_manifest.json")
	if err != nil {
		return DownloadResult{}, fmt.Errorf("fetch version manifest: %w", err)
	}
	versions, ok := manifest["versions"].([]interface{})
	if !ok {
		return DownloadResult{}, errors.New("invalid version manifest")
	}
	var versionURL string
	for _, item := range versions {
		vm, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if id, _ := vm["id"].(string); id == mcVersion {
			versionURL, _ = vm["url"].(string)
			break
		}
	}
	if versionURL == "" {
		return DownloadResult{}, fmt.Errorf("vanilla version %s not found in manifest", mcVersion)
	}
	versionMeta, err := fetchJSON(ctx, versionURL)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("fetch version metadata: %w", err)
	}
	downloads, ok := versionMeta["downloads"].(map[string]interface{})
	if !ok {
		return DownloadResult{}, errors.New("invalid version metadata")
	}
	server, ok := downloads["server"].(map[string]interface{})
	if !ok {
		return DownloadResult{}, errors.New("server download not found")
	}
	jarURL, _ := server["url"].(string)
	if jarURL == "" {
		return DownloadResult{}, errors.New("server jar URL not found")
	}
	jarName := "server-" + mcVersion + ".jar"
	return d.downloadFile(ctx, jarURL, jarName)
}

func (d *Downloader) downloadFabric(ctx context.Context, mcVersion, loaderVersion string) (DownloadResult, error) {
	if loaderVersion == "" || loaderVersion == "latest" {
		return d.downloadFabricLatest(ctx, mcVersion)
	}
	loaderURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s", mcVersion, loaderVersion)
	loaderEntry, err := fetchJSON(ctx, loaderURL)
	if err != nil {
		return d.downloadFabricLatest(ctx, mcVersion)
	}
	installerVersion := "1.0.1"
	if installer, ok := loaderEntry["installer"].(map[string]interface{}); ok {
		if iv, _ := installer["version"].(string); iv != "" {
			installerVersion = iv
		}
	}
	loader, ok := loaderEntry["loader"].(map[string]interface{})
	loaderVer := loaderVersion
	if ok {
		if v, _ := loader["version"].(string); v != "" {
			loaderVer = v
		}
	}
	url := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/%s/server/jar", mcVersion, loaderVersion, installerVersion)
	jarName := fmt.Sprintf("fabric-server-%s-%s.jar", mcVersion, loaderVer)
	result, err := d.downloadFile(ctx, url, jarName)
	if err != nil {
		return d.downloadFabricLatest(ctx, mcVersion)
	}
	return result, nil
}

func (d *Downloader) downloadFabricLatest(ctx context.Context, mcVersion string) (DownloadResult, error) {
	listURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s", mcVersion)
	data, err := fetchJSONAsArray(ctx, listURL)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("fetch Fabric loader list: %w", err)
	}
	for _, entry := range data {
		loader, ok := entry["loader"].(map[string]interface{})
		if !ok {
			continue
		}
		lv, _ := loader["version"].(string)
		if lv == "" {
			continue
		}
		installerVersion := "1.0.1"
		if installer, ok := entry["installer"].(map[string]interface{}); ok {
			if iv, _ := installer["version"].(string); iv != "" {
				installerVersion = iv
			}
		}
		url := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/%s/server/jar", mcVersion, lv, installerVersion)
		jarName := fmt.Sprintf("fabric-server-%s-%s.jar", mcVersion, lv)
		result, err := d.downloadFile(ctx, url, jarName)
		if err != nil {
			continue
		}
		return result, nil
	}
	return DownloadResult{}, fmt.Errorf("no compatible Fabric loader found for Minecraft %s", mcVersion)
}

func fetchJSONAsArray(ctx context.Context, urlStr string) ([]map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, urlStr)
	}
	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode JSON from %s: %w", urlStr, err)
	}
	return result, nil
}

func (d *Downloader) downloadPaper(ctx context.Context, mcVersion string) (DownloadResult, error) {
	buildsURL := fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s", mcVersion)
	data, err := fetchJSON(ctx, buildsURL)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("fetch Paper versions: %w", err)
	}
	buildsList, ok := data["builds"].([]interface{})
	if !ok || len(buildsList) == 0 {
		return DownloadResult{}, fmt.Errorf("no Paper builds for version %s", mcVersion)
	}
	lastBuild := buildsList[len(buildsList)-1]
	buildNum, ok := lastBuild.(float64)
	if !ok {
		return DownloadResult{}, fmt.Errorf("invalid Paper build number")
	}
	build := int(buildNum)
	downloadURL := fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s/builds/%d/downloads/paper-%s-%d.jar", mcVersion, build, mcVersion, build)
	jarName := fmt.Sprintf("paper-%s-%d.jar", mcVersion, build)
	result, err := d.downloadFile(ctx, downloadURL, jarName)
	if err != nil {
		return DownloadResult{}, err
	}
	result.Version = fmt.Sprintf("%s-b%d", mcVersion, build)
	return result, nil
}

func (d *Downloader) downloadFile(ctx context.Context, fileURL, fileName string) (DownloadResult, error) {
	if err := os.MkdirAll(d.CacheDir, 0o755); err != nil {
		return DownloadResult{}, fmt.Errorf("create cache directory: %w", err)
	}
	destPath := filepath.Join(d.CacheDir, fileName)
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		hash, err := fileSHA256(destPath)
		if err == nil {
			return DownloadResult{JarPath: destPath, JarName: fileName, SHA256: hash, SizeBytes: info.Size()}, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("create request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("download %s: %w", fileURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DownloadResult{}, fmt.Errorf("download %s returned status %d", fileURL, resp.StatusCode)
	}
	tmp, err := os.CreateTemp(d.CacheDir, ".download-*.tmp")
	if err != nil {
		return DownloadResult{}, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	hasher := sha256.New()
	writer := io.MultiWriter(tmp, hasher)
	size, err := io.Copy(writer, resp.Body)
	if err != nil {
		tmp.Close()
		return DownloadResult{}, fmt.Errorf("write download: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return DownloadResult{}, fmt.Errorf("sync download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return DownloadResult{}, fmt.Errorf("close download: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return DownloadResult{}, fmt.Errorf("rename download: %w", err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	return DownloadResult{JarPath: destPath, JarName: fileName, SHA256: hash, SizeBytes: size}, nil
}

func fetchJSON(ctx context.Context, urlStr string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, urlStr)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode JSON from %s: %w", urlStr, err)
	}
	return result, nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".copy-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, srcFile); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, dst)
}
