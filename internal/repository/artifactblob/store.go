package artifactblob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	stratumerrors "github.com/stratummc/stratum/internal/errors"
)

const (
	Algorithm            = "sha256"
	directoryPermissions = 0o750
	blobPermissions      = 0o640
)

type Metadata struct {
	Algorithm string
	Hash      string
	Size      int64
	Reference string
	CreatedAt time.Time
}

type Store struct {
	Root             string
	BlobRoot         string
	tempRoot         string
	resolvedBlobRoot string
}

func New(root string) (*Store, error) {
	const operation = "artifactblob.New"
	if strings.TrimSpace(root) == "" {
		return nil, validationError(operation, "artifact blob storage root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, storageError(operation, "resolve artifact blob storage root", err)
	}
	blobRoot := filepath.Join(filepath.Clean(absoluteRoot), "blobs")
	store := &Store{Root: filepath.Clean(absoluteRoot), BlobRoot: blobRoot, tempRoot: filepath.Join(blobRoot, ".tmp")}
	for _, directory := range []string{filepath.Join(blobRoot, Algorithm), store.tempRoot} {
		if err := os.MkdirAll(directory, directoryPermissions); err != nil {
			return nil, storageError(operation, "create artifact blob directory", err)
		}
	}
	store.resolvedBlobRoot, err = filepath.EvalSymlinks(blobRoot)
	if err != nil {
		return nil, storageError(operation, "resolve artifact blob directory", err)
	}
	return store, nil
}

func ComputeSHA256(reader io.Reader) (hash string, size int64, err error) {
	if reader == nil {
		return "", 0, validationError("artifactblob.ComputeSHA256", "blob reader is required")
	}
	digest := sha256.New()
	size, err = io.Copy(digest, reader)
	if err != nil {
		return "", 0, storageError("artifactblob.ComputeSHA256", "hash blob", err)
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}

func ComputeFileSHA256(path string) (hash string, size int64, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, storageError("artifactblob.ComputeFileSHA256", "open blob source", err)
	}
	defer file.Close()
	return ComputeSHA256(file)
}

func (s *Store) Put(ctx context.Context, reader io.Reader) (Metadata, error) {
	const operation = "artifactblob.Put"
	if reader == nil {
		return Metadata{}, validationError(operation, "blob reader is required")
	}
	if err := ctx.Err(); err != nil {
		return Metadata{}, storageError(operation, "store blob", err)
	}
	temporary, err := os.CreateTemp(s.tempRoot, ".stratum-blob-*.tmp")
	if err != nil {
		return Metadata{}, storageError(operation, "create temporary blob", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	digest := sha256.New()
	size, err := io.Copy(io.MultiWriter(temporary, digest), reader)
	if err != nil {
		_ = temporary.Close()
		return Metadata{}, storageError(operation, "write temporary blob", err)
	}
	if err := ctx.Err(); err != nil {
		_ = temporary.Close()
		return Metadata{}, storageError(operation, "store blob", err)
	}
	if err := temporary.Chmod(blobPermissions); err != nil {
		_ = temporary.Close()
		return Metadata{}, storageError(operation, "set blob permissions", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return Metadata{}, storageError(operation, "sync temporary blob", err)
	}
	if err := temporary.Close(); err != nil {
		return Metadata{}, storageError(operation, "close temporary blob", err)
	}

	hash := hex.EncodeToString(digest.Sum(nil))
	target, err := s.Path(hash)
	if err != nil {
		return Metadata{}, err
	}
	if err := s.ensureContentDirectory(filepath.Dir(target)); err != nil {
		return Metadata{}, err
	}
	if _, err := os.Stat(target); err == nil {
		return s.Verify(ctx, hash)
	} else if !os.IsNotExist(err) {
		return Metadata{}, storageError(operation, "inspect existing blob", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			return s.Verify(ctx, hash)
		}
		return Metadata{}, storageError(operation, "commit content-addressed blob", err)
	}
	return s.metadata(target, hash, size)
}

func (s *Store) PutFile(ctx context.Context, path string) (Metadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}, storageError("artifactblob.PutFile", "open blob source", err)
	}
	defer file.Close()
	return s.Put(ctx, file)
}

func (s *Store) HashFile(path string) (algorithm, hash string, size int64, err error) {
	hash, size, err = ComputeFileSHA256(path)
	return Algorithm, hash, size, err
}

func (s *Store) StoreFile(ctx context.Context, path string) (algorithm, hash, reference string, size int64, err error) {
	metadata, err := s.PutFile(ctx, path)
	if err != nil {
		return "", "", "", 0, err
	}
	return metadata.Algorithm, metadata.Hash, metadata.Reference, metadata.Size, nil
}

func (s *Store) Verify(ctx context.Context, hash string) (Metadata, error) {
	const operation = "artifactblob.Verify"
	path, err := s.Path(hash)
	if err != nil {
		return Metadata{}, err
	}
	if err := ctx.Err(); err != nil {
		return Metadata{}, storageError(operation, "verify blob", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, stratumerrors.Error{Kind: stratumerrors.KindNotFound, Operation: operation, Message: "blob does not exist"}
		}
		return Metadata{}, storageError(operation, "inspect blob", err)
	}
	if !info.Mode().IsRegular() {
		return Metadata{}, storageError(operation, "blob path is not a regular file", nil)
	}
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}, storageError(operation, "open blob", err)
	}
	actualHash, size, hashErr := ComputeSHA256(file)
	closeErr := file.Close()
	if hashErr != nil {
		return Metadata{}, hashErr
	}
	if closeErr != nil {
		return Metadata{}, storageError(operation, "close blob", closeErr)
	}
	if actualHash != hash {
		return Metadata{}, storageError(operation, fmt.Sprintf("blob hash mismatch: expected %s, got %s", hash, actualHash), nil)
	}
	return s.metadata(path, hash, size)
}

func (s *Store) Path(hash string) (string, error) {
	const operation = "artifactblob.Path"
	if err := validateHash(hash); err != nil {
		return "", validationError(operation, err.Error())
	}
	path := filepath.Join(s.BlobRoot, Algorithm, hash[:2], hash)
	relative, err := filepath.Rel(s.BlobRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", validationError(operation, "blob path escapes storage root")
	}
	return path, nil
}

func (s *Store) metadata(path, hash string, size int64) (Metadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Metadata{}, storageError("artifactblob.metadata", "inspect blob", err)
	}
	reference := filepath.ToSlash(filepath.Join(Algorithm, hash[:2], hash))
	return Metadata{Algorithm: Algorithm, Hash: hash, Size: size, Reference: reference, CreatedAt: info.ModTime().UTC()}, nil
}

func (s *Store) ensureContentDirectory(directory string) error {
	const operation = "artifactblob.ensureContentDirectory"
	if err := os.MkdirAll(directory, directoryPermissions); err != nil {
		return storageError(operation, "create content-addressed directory", err)
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return storageError(operation, "resolve content-addressed directory", err)
	}
	relative, err := filepath.Rel(s.resolvedBlobRoot, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return validationError(operation, "content-addressed directory escapes blob root")
	}
	return nil
}

func validateHash(hash string) error {
	if len(hash) != sha256.Size*2 {
		return fmt.Errorf("SHA-256 hash must contain 64 lowercase hexadecimal characters")
	}
	for _, character := range hash {
		if (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') {
			continue
		}
		return fmt.Errorf("SHA-256 hash must contain 64 lowercase hexadecimal characters")
	}
	return nil
}

func validationError(operation, message string) error {
	return stratumerrors.Error{Kind: stratumerrors.KindValidation, Operation: operation, Message: message}
}

func storageError(operation, message string, cause error) error {
	return stratumerrors.Error{Kind: stratumerrors.KindConflict, Operation: operation, Message: message, Cause: cause}
}
