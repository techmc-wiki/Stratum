package worldcheckpoint

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent/process"
)

type CreateParams struct {
	SessionRoot string
	WorldDir    string
}

type Result struct {
	SnapshotRef string
	SizeBytes   int64
	SHA256      string
	CreatedAt   time.Time
}

type RestoreParams struct {
	SessionRoot  string
	WorldDirRel  string
	SnapshotPath string
}

type RestoreResult struct {
	RestoredDir string
	EntryCount  int
	SizeBytes   int64
}

type Worker struct {
	runtimeRoot string
}

func NewWorker(runtimeRoot string) (*Worker, error) {
	abs, err := filepath.Abs(filepath.Clean(runtimeRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve runtime root: %w", err)
	}
	return &Worker{runtimeRoot: abs}, nil
}

func (w *Worker) Create(ctx context.Context, params CreateParams) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(params.SessionRoot) == "" || strings.TrimSpace(params.WorldDir) == "" {
		return Result{}, errors.New("session root and world dir are required")
	}
	sessionRoot, err := filepath.Abs(filepath.Clean(params.SessionRoot))
	if err != nil {
		return Result{}, fmt.Errorf("resolve session root: %w", err)
	}
	if !pathWithin(w.runtimeRoot, sessionRoot) {
		return Result{}, fmt.Errorf("session root %q escapes runtime root", sessionRoot)
	}
	worldDir, err := filepath.Abs(filepath.Clean(params.WorldDir))
	if err != nil {
		return Result{}, fmt.Errorf("resolve world dir: %w", err)
	}
	if !pathWithin(sessionRoot, worldDir) {
		return Result{}, fmt.Errorf("world dir %q escapes session root", worldDir)
	}
	worldInfo, err := os.Stat(worldDir)
	if err != nil {
		return Result{}, fmt.Errorf("inspect world dir: %w", err)
	}
	if !worldInfo.IsDir() {
		return Result{}, errors.New("world dir is not a directory")
	}
	layout, err := process.NewSessionRuntimeLayout(w.runtimeRoot, filepath.Base(sessionRoot))
	if err != nil {
		return Result{}, fmt.Errorf("build session layout: %w", err)
	}
	snapshotDir := layout.CheckpointsDir
	if !pathWithin(sessionRoot, snapshotDir) {
		return Result{}, fmt.Errorf("snapshot dir %q escapes session root", snapshotDir)
	}
	if err := os.MkdirAll(snapshotDir, 0o750); err != nil {
		return Result{}, fmt.Errorf("create snapshot directory: %w", err)
	}
	createdAt := time.Now().UTC()
	snapshotName := fmt.Sprintf("world-%s.zip", createdAt.Format("20060102T150405Z"))
	snapshotPath := filepath.Join(snapshotDir, snapshotName)
	if !pathWithin(sessionRoot, snapshotPath) {
		return Result{}, fmt.Errorf("snapshot path %q escapes session root", snapshotPath)
	}
	f, err := os.Create(snapshotPath)
	if err != nil {
		return Result{}, fmt.Errorf("create snapshot file: %w", err)
	}
	defer f.Close()
	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)
	zipWriter := zip.NewWriter(writer)
	if err := w.writeDir(zipWriter, worldDir, worldDir); err != nil {
		zipWriter.Close()
		f.Close()
		os.Remove(snapshotPath)
		return Result{}, fmt.Errorf("write snapshot archive: %w", err)
	}
	if err := zipWriter.Close(); err != nil {
		f.Close()
		os.Remove(snapshotPath)
		return Result{}, fmt.Errorf("close snapshot archive: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(snapshotPath)
		return Result{}, fmt.Errorf("close snapshot file: %w", err)
	}
	info, err := os.Stat(snapshotPath)
	if err != nil {
		return Result{}, fmt.Errorf("inspect snapshot: %w", err)
	}
	return Result{
		SnapshotRef: snapshotPath,
		SizeBytes:   info.Size(),
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
		CreatedAt:   createdAt,
	}, nil
}

func (w *Worker) Restore(ctx context.Context, params RestoreParams) (RestoreResult, error) {
	if err := ctx.Err(); err != nil {
		return RestoreResult{}, err
	}
	if strings.TrimSpace(params.SessionRoot) == "" {
		return RestoreResult{}, errors.New("session root is required")
	}
	if strings.TrimSpace(params.SnapshotPath) == "" {
		return RestoreResult{}, errors.New("snapshot path is required")
	}
	sessionRoot, err := filepath.Abs(filepath.Clean(params.SessionRoot))
	if err != nil {
		return RestoreResult{}, fmt.Errorf("resolve session root: %w", err)
	}
	if !pathWithin(w.runtimeRoot, sessionRoot) {
		return RestoreResult{}, fmt.Errorf("session root %q escapes runtime root", sessionRoot)
	}
	worldRel := strings.TrimSpace(params.WorldDirRel)
	if worldRel == "" {
		worldRel = "world"
	}
	if filepath.IsAbs(worldRel) || strings.Contains(worldRel, "..") || worldRel == "." {
		return RestoreResult{}, errors.New("world dir relative path must be safe")
	}
	targetDir := filepath.Join(sessionRoot, "work", worldRel)
	targetDir, err = filepath.Abs(filepath.Clean(targetDir))
	if err != nil {
		return RestoreResult{}, fmt.Errorf("resolve target dir: %w", err)
	}
	if !pathWithin(sessionRoot, targetDir) {
		return RestoreResult{}, fmt.Errorf("target dir %q escapes session root", targetDir)
	}
	snapshotPath, err := filepath.Abs(filepath.Clean(params.SnapshotPath))
	if err != nil {
		return RestoreResult{}, fmt.Errorf("resolve snapshot path: %w", err)
	}
	reader, err := zip.OpenReader(snapshotPath)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("open snapshot zip: %w", err)
	}
	defer reader.Close()
	var created []string
	defer func() {
		if len(created) > 0 {
			for i := len(created) - 1; i >= 0; i-- {
				os.Remove(created[i])
			}
		}
	}()
	var entryCount int
	var totalBytes int64
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return RestoreResult{}, err
		}
		if entry.Mode().Type()&os.ModeSymlink != 0 {
			return RestoreResult{}, fmt.Errorf("symlink rejected: %s", entry.Name)
		}
		if entry.Name == "" {
			return RestoreResult{}, errors.New("zip entry with empty name is rejected")
		}
		if strings.HasPrefix(entry.Name, "/") {
			return RestoreResult{}, fmt.Errorf("absolute path rejected: %s", entry.Name)
		}
		if strings.Contains(entry.Name, "..") {
			return RestoreResult{}, fmt.Errorf("path traversal rejected: %s", entry.Name)
		}
		fullPath := filepath.Join(targetDir, filepath.FromSlash(entry.Name))
		fullPath = filepath.Clean(fullPath)
		if !pathWithin(targetDir, fullPath) {
			return RestoreResult{}, fmt.Errorf("zip entry %q escapes target dir", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(fullPath, 0o750); err != nil {
				return RestoreResult{}, fmt.Errorf("create directory %s: %w", entry.Name, err)
			}
			created = append(created, fullPath)
			continue
		}
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return RestoreResult{}, fmt.Errorf("create parent directory for %s: %w", entry.Name, err)
		}
		created = append(created, dir)
		rc, err := entry.Open()
		if err != nil {
			return RestoreResult{}, fmt.Errorf("open zip entry %s: %w", entry.Name, err)
		}
		dst, err := os.Create(fullPath)
		if err != nil {
			rc.Close()
			return RestoreResult{}, fmt.Errorf("create file %s: %w", entry.Name, err)
		}
		written, err := io.Copy(dst, rc)
		rc.Close()
		dst.Close()
		if err != nil {
			return RestoreResult{}, fmt.Errorf("write file %s: %w", entry.Name, err)
		}
		created = append(created, fullPath)
		totalBytes += written
		entryCount++
	}
	created = nil
	return RestoreResult{
		RestoredDir: targetDir,
		EntryCount:  entryCount,
		SizeBytes:   totalBytes,
	}, nil
}

func (w *Worker) writeDir(zipWriter *zip.Writer, baseDir, targetDir string) error {
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fullPath := filepath.Join(targetDir, entry.Name())
		if !pathWithin(baseDir, fullPath) {
			continue
		}
		relPath, err := filepath.Rel(baseDir, fullPath)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		if entry.IsDir() {
			dirHeader := &zip.FileHeader{Name: relPath + "/", Method: zip.Deflate}
			dirHeader.SetMode(0o750)
			if _, err := zipWriter.CreateHeader(dirHeader); err != nil {
				return err
			}
			if err := w.writeDir(zipWriter, baseDir, fullPath); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate
		w, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		src, err := os.Open(fullPath)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, src)
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	return candidate == root || strings.HasPrefix(candidate, root+string(filepath.Separator))
}
