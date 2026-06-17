package lucy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	lucystate "github.com/mclucy/lucy/state"
)

// LockService provides operations for Lucy lock file management.
type LockService struct {
	workDir string
}

// NewLockService creates a LockService for the given work directory.
func NewLockService(workDir string) *LockService {
	return &LockService{workDir: workDir}
}

// Read reads the Lucy lock file from the work directory.
func (s *LockService) Read(ctx context.Context) (*lucystate.Lock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	svc := lucystate.NewProjectStateService(s.workDir)
	if err := svc.Load(ctx); err != nil {
		return nil, fmt.Errorf("load lucy state: %w", err)
	}
	return svc.Lock(), nil
}

// Write writes a Lucy lock file to the work directory.
func (s *LockService) Write(ctx context.Context, lock *lucystate.Lock) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	svc := lucystate.NewProjectStateService(s.workDir)
	if err := svc.Save(ctx, nil, nil, lock); err != nil {
		return fmt.Errorf("save lock: %w", err)
	}
	return nil
}

// Hash computes the SHA-256 hash of a serialized lock file for checkpoint metadata.
func Hash(lock *lucystate.Lock) (string, error) {
	if lock == nil {
		return "", nil
	}
	data, err := lucystate.SerializeLock(lock)
	if err != nil {
		return "", fmt.Errorf("serialize lock: %w", err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
