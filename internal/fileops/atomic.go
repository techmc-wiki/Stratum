package fileops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func WriteFileAtomic(path string, data []byte, permissions, directoryPermissions os.FileMode, tempPattern string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, directoryPermissions); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, tempPattern)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(permissions); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}

func WriteJSONAtomic(path string, value any, permissions, directoryPermissions os.FileMode, tempPattern string) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize JSON: %w", err)
	}
	payload = append(payload, '\n')
	return WriteFileAtomic(path, payload, permissions, directoryPermissions, tempPattern)
}

func SHA256Hex(path string) (string, error) {
	digest, _, err := SHA256HexWithSize(path)
	return digest, err
}

func SHA256HexWithSize(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}
