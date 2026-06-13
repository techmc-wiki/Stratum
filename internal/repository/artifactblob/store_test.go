package artifactblob

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	stratumerrors "github.com/stratummc/stratum/internal/errors"
)

func TestComputeSHA256(t *testing.T) {
	hash, size, err := ComputeSHA256(strings.NewReader("stratum blob"))
	if err != nil {
		t.Fatal(err)
	}
	if hash != "8009e582f2ca44b7bf4cf67e16ceb08f1703f3c33822718112bd0834b48dd7ed" || size != 12 {
		t.Fatalf("hash=%q size=%d", hash, size)
	}
}

func TestPutWritesContentAddressedBlobAndCreatesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifact-storage")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("artifact payload")
	metadata, err := store.Put(context.Background(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "blobs", "sha256", metadata.Hash[:2], metadata.Hash)
	path, err := store.Path(metadata.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if path != wantPath || metadata.Algorithm != Algorithm || metadata.Size != int64(len(payload)) || metadata.Reference != filepath.ToSlash(filepath.Join("sha256", metadata.Hash[:2], metadata.Hash)) || metadata.CreatedAt.IsZero() {
		t.Fatalf("path=%q metadata=%+v", path, metadata)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, payload) {
		t.Fatalf("stored payload=%q", stored)
	}
	for _, directory := range []string{store.BlobRoot, filepath.Join(store.BlobRoot, Algorithm), filepath.Join(store.BlobRoot, ".tmp")} {
		if info, err := os.Stat(directory); err != nil || !info.IsDir() {
			t.Fatalf("directory %q info=%v err=%v", directory, info, err)
		}
	}
}

func TestPutDuplicateIsIdempotent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("same payload")
	first, err := store.Put(context.Background(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put(context.Background(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash != second.Hash || first.Size != second.Size || first.Reference != second.Reference || !first.CreatedAt.Equal(second.CreatedAt) {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestVerifyIntactAndCorruptedBlob(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Put(context.Background(), strings.NewReader("trusted payload"))
	if err != nil {
		t.Fatal(err)
	}
	verified, err := store.Verify(context.Background(), metadata.Hash)
	if err != nil || verified.Hash != metadata.Hash || verified.Size != metadata.Size {
		t.Fatalf("verified=%+v err=%v", verified, err)
	}
	path, _ := store.Path(metadata.Hash)
	if err := os.WriteFile(path, []byte("corrupted"), blobPermissions); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Verify(context.Background(), metadata.Hash); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("corruption err=%v", err)
	}
	if _, err := store.Put(context.Background(), strings.NewReader("trusted payload")); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("duplicate corrupted blob err=%v", err)
	}
}

func TestPathRejectsInvalidHashesAndCannotEscapeRoot(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	invalid := []string{"", "abc", strings.Repeat("A", 64), strings.Repeat("0", 63) + "/", "../" + strings.Repeat("0", 61)}
	for _, hash := range invalid {
		if _, err := store.Path(hash); err == nil || !stratumerrors.IsKind(err, stratumerrors.KindValidation) {
			t.Fatalf("hash %q err=%v", hash, err)
		}
	}
	valid := strings.Repeat("a", 64)
	path, err := store.Path(valid)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(store.BlobRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("path escaped root: root=%q path=%q relative=%q err=%v", store.BlobRoot, path, relative, err)
	}
}

func TestPutFileComputesContentAddress(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "artifact.jar")
	if err := os.WriteFile(source, []byte("file payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, err := store.PutFile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := store.Verify(context.Background(), metadata.Hash)
	if err != nil || verified.Size != int64(len("file payload")) {
		t.Fatalf("verified=%+v err=%v", verified, err)
	}
}
