package lucy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func expectedPackageFilename(pkg LockedPackage) string {
	name := strings.TrimSpace(pkg.Name)
	if name == "" {
		name = packageNameFromID(pkg.ID)
	}
	version := strings.TrimSpace(pkg.Version)
	if version == "" {
		return name + ".jar"
	}
	return name + "-" + version + ".jar"
}

func packageNameFromID(id string) string {
	if _, name, ok := strings.Cut(id, "/"); ok {
		return name
	}
	return id
}

func packagePlatformFromID(id string) string {
	if platform, _, ok := strings.Cut(id, "/"); ok {
		return platform
	}
	return ""
}

func fillInstallPaths(req InstallPackagesRequest, result *InstallPackagesResult) {
	if result == nil {
		return
	}
	lockedByID := make(map[string]LockedPackage, len(req.Packages))
	for _, pkg := range req.Packages {
		lockedByID[pkg.ID] = pkg
	}
	for i := range result.Installed {
		installed := &result.Installed[i]
		if installed.Path == "" {
			if locked, ok := lockedByID[installed.ID]; ok {
				installed.Path = filepath.Join(req.TargetDir, expectedPackageFilename(locked))
			} else {
				installed.Path = filepath.Join(req.TargetDir, installed.Name+"-"+installed.Version+".jar")
			}
		}
		if installed.Hash == "" {
			if locked, ok := lockedByID[installed.ID]; ok {
				installed.Hash = locked.Hash
			}
		}
	}
}

func validateInstalledHashes(req InstallPackagesRequest, result InstallPackagesResult) InstallPackagesResult {
	lockedByID := make(map[string]LockedPackage, len(req.Packages))
	for _, pkg := range req.Packages {
		lockedByID[pkg.ID] = pkg
	}
	validated := InstallPackagesResult{
		Installed: make([]InstalledPackage, 0, len(result.Installed)),
		Failed:    append([]FailedPackage(nil), result.Failed...),
		Status:    result.Status,
		TotalSize: result.TotalSize,
	}
	for _, installed := range result.Installed {
		locked, ok := lockedByID[installed.ID]
		if !ok || strings.TrimSpace(locked.Hash) == "" {
			validated.Installed = append(validated.Installed, installed)
			continue
		}
		path := installed.Path
		if path == "" {
			path = filepath.Join(req.TargetDir, expectedPackageFilename(locked))
		}
		actual, err := fileSHA256(path)
		if err != nil {
			if foundPath, foundErr := findFileBySHA256(req.TargetDir, locked.Hash); foundErr == nil {
				path = foundPath
				actual = normalizeHash(locked.Hash)
			} else {
				validated.Failed = append(validated.Failed, FailedPackage{ID: installed.ID, Error: err.Error()})
				continue
			}
		}
		if normalizeHash(locked.Hash) != "" && normalizeHash(locked.Hash) != actual {
			validated.Failed = append(validated.Failed, FailedPackage{ID: installed.ID, Error: "hash mismatch"})
			continue
		}
		installed.Path = path
		installed.Hash = locked.Hash
		validated.Installed = append(validated.Installed, installed)
	}
	validated.Status = installStatus(len(validated.Installed), len(validated.Failed), len(req.Packages))
	return validated
}

func installStatus(installed, failed, requested int) string {
	if requested == 0 {
		return "ok"
	}
	if failed == 0 && installed == requested {
		return "ok"
	}
	if installed > 0 {
		return "partial"
	}
	return "failed"
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func findFileBySHA256(root, expected string) (string, error) {
	needle := normalizeHash(expected)
	if needle == "" {
		return "", fmt.Errorf("empty hash")
	}
	var match string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || match != "" {
			return err
		}
		actual, err := fileSHA256(path)
		if err != nil {
			return nil
		}
		if actual == needle {
			match = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if match == "" {
		return "", fmt.Errorf("no file matching hash %s", expected)
	}
	return match, nil
}

func normalizeHash(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if _, hash, ok := strings.Cut(value, ":"); ok {
		return hash
	}
	return value
}
