package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/artifact"
	artifactsvc "github.com/stratummc/stratum/internal/artifact/service"
	"github.com/stratummc/stratum/internal/storage/artifactblob"
	"github.com/stratummc/stratum/internal/storage/filesystem"
	stratumerrors "github.com/stratummc/stratum/internal/stratumerr"
)

func listArtifacts(ctx context.Context, store *filesystem.Store, stdout, stderr io.Writer) int {
	values, err := store.ListArtifacts(ctx)
	if err != nil {
		return reportError(stderr, "list artifacts", err)
	}
	for _, value := range values {
		reviewedAt := ""
		if value.ReviewedAt != nil {
			reviewedAt = value.ReviewedAt.Format(time.RFC3339Nano)
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\treviewedBy=%s\treviewedAt=%s\treviewReason=%s\tproject=%s\tpayload=%s\n", value.ID, value.Name, value.Type, value.Status, value.ReviewedBy, reviewedAt, value.ReviewReason, value.ProjectID, value.PayloadStatus)
	}
	return 0
}

func inspectArtifact(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts inspect", stderr)
	id := flags.String("id", "", "artifact ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	value, err := store.GetArtifact(ctx, *id)
	if err != nil {
		if stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
			err = fmt.Errorf("artifact %q not found: %w", *id, err)
		}
		return reportError(stderr, "inspect artifact", err)
	}
	fmt.Fprintf(stdout, "id=%s name=%q type=%s status=%s uploadedBy=%s createdAt=%s",
		value.ID, value.Name, value.Type, value.Status, value.UploaderID, value.CreatedAt.Format(time.RFC3339Nano))
	if value.ProjectID != "" {
		fmt.Fprintf(stdout, " project=%s", value.ProjectID)
	}
	if value.ReviewedBy != "" {
		fmt.Fprintf(stdout, " reviewedBy=%s", value.ReviewedBy)
	}
	if value.ReviewedAt != nil {
		fmt.Fprintf(stdout, " reviewedAt=%s", value.ReviewedAt.Format(time.RFC3339Nano))
	}
	if value.ReviewReason != "" {
		fmt.Fprintf(stdout, " reviewReason=%q", value.ReviewReason)
	}
	if value.PayloadStatus != "" {
		fmt.Fprintf(stdout, " payload=%s", value.PayloadStatus)
	}
	if value.PayloadAlgorithm != "" {
		fmt.Fprintf(stdout, " payloadAlgorithm=%s", value.PayloadAlgorithm)
	}
	if value.SHA256 != "" {
		fmt.Fprintf(stdout, " hash=%s size=%d", value.SHA256, value.SizeBytes)
	}
	if value.PayloadReference != "" {
		fmt.Fprintf(stdout, " payloadReference=%s", value.PayloadReference)
	}
	if value.PayloadImportedBy != "" {
		fmt.Fprintf(stdout, " payloadImportedBy=%s", value.PayloadImportedBy)
	}
	if value.PayloadImportedAt != nil {
		fmt.Fprintf(stdout, " payloadImportedAt=%s", value.PayloadImportedAt.Format(time.RFC3339Nano))
	}
	if len(value.TargetMinecraftVersions) > 0 {
		fmt.Fprintf(stdout, " targetVersions=%s", strings.Join(value.TargetMinecraftVersions, ","))
	}
	if len(value.LoaderCompatibility) > 0 {
		fmt.Fprintf(stdout, " loaders=%s", strings.Join(value.LoaderCompatibility, ","))
	}
	fmt.Fprintln(stdout)
	return 0
}

func createArtifact(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts create", stderr)
	id := flags.String("id", "", "artifact ID")
	name := flags.String("name", "", "artifact name")
	typeValue := flags.String("type", "", "artifact type")
	projectID := flags.String("project", "", "project ID")
	actor := flags.String("actor", "", "creator actor ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || strings.TrimSpace(*name) == "" || strings.TrimSpace(*typeValue) == "" || *projectID == "" || strings.TrimSpace(*actor) == "" {
		fmt.Fprintln(stderr, "--id, --name, --type, --project, and --actor are required")
		return 2
	}
	value, err := artifactsvc.New(store).CreateMetadata(ctx, *id, *name, artifact.Type(*typeValue), *projectID, *actor)
	if err != nil {
		return reportError(stderr, "create artifact", err)
	}
	fmt.Fprintf(stdout, "Artifact %s name=%q type=%s status=%s project=%s payload=%s. Metadata only; no payload was uploaded, hashed, copied, mounted, installed, or executed.\n",
		value.ID, value.Name, value.Type, value.Status, value.ProjectID, value.PayloadStatus)
	return 0
}

func importArtifactFile(ctx context.Context, store *filesystem.Store, blobRoot string, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts import-file", stderr)
	id := flags.String("id", "", "artifact ID")
	path := flags.String("file", "", "local artifact payload file")
	actor := flags.String("actor", "", "importing actor ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || strings.TrimSpace(*path) == "" || strings.TrimSpace(*actor) == "" {
		fmt.Fprintln(stderr, "--id, --file, and --actor are required")
		return 2
	}
	blobs, err := artifactblob.New(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	value, err := artifactsvc.NewWithBlobStore(store, blobs).ImportFile(ctx, *id, *path, *actor)
	if err != nil {
		return reportError(stderr, "import artifact payload", err)
	}
	fmt.Fprintf(stdout, "Artifact %s payloadAlgorithm=%s payloadHash=%s payloadSize=%d payloadStatus=%s. The artifact remains %s and was not approved, mounted, installed, or executed.\n",
		value.ID, value.PayloadAlgorithm, value.SHA256, value.SizeBytes, value.PayloadStatus, value.Status)
	return 0
}

func artifactBlobs(ctx context.Context, blobRoot string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "verify" {
		fmt.Fprintln(stderr, "usage: stratum artifacts blobs verify --sha256 HASH")
		return 2
	}
	flags := newFlagSet("artifacts blobs verify", stderr)
	hash := flags.String("sha256", "", "SHA-256 content hash")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *hash == "" {
		fmt.Fprintln(stderr, "--sha256 is required")
		return 2
	}
	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	metadata, err := blobs.Verify(ctx, *hash)
	if err != nil {
		if stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
			fmt.Fprintf(stderr, "algorithm=sha256 hash=%s status=missing\n", *hash)
		} else if stratumerrors.IsKind(err, stratumerrors.KindValidation) {
			return reportError(stderr, "verify artifact blob", err)
		} else if strings.Contains(err.Error(), "hash mismatch") {
			fmt.Fprintf(stderr, "algorithm=sha256 hash=%s status=corrupted\n", *hash)
		}
		return reportError(stderr, "verify artifact blob", err)
	}
	fmt.Fprintf(stdout, "algorithm=%s hash=%s size=%d status=valid reference=%s\n", metadata.Algorithm, metadata.Hash, metadata.Size, metadata.Reference)
	return 0
}

func reviewArtifact(ctx context.Context, store *filesystem.Store, blobRoot, action string, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts "+action, stderr)
	id := flags.String("id", "", "artifact ID")
	actor := flags.String("actor", "", "reviewer actor ID")
	reason := flags.String("reason", "", "review reason")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || strings.TrimSpace(*actor) == "" || strings.TrimSpace(*reason) == "" {
		fmt.Fprintln(stderr, "--id, --actor, and --reason are required")
		return 2
	}
	service := artifactsvc.New(store)
	var value artifact.Artifact
	var err error
	if action == "approve" {
		blobs, openErr := artifactblob.Open(blobRoot)
		if openErr != nil {
			return reportError(stderr, "open artifact blob store", openErr)
		}
		service = artifactsvc.NewWithPayloadVerifier(store, blobs)
		value, err = service.ApproveArtifact(ctx, *id, *actor, *reason)
	} else {
		value, err = service.RejectArtifact(ctx, *id, *actor, *reason)
	}
	if err != nil {
		return reportError(stderr, "artifact "+action, err)
	}
	reviewedAt := ""
	if value.ReviewedAt != nil {
		reviewedAt = value.ReviewedAt.Format(time.RFC3339Nano)
	}
	fmt.Fprintf(stdout, "Artifact %s status=%s reviewedBy=%s reviewedAt=%s reason=%q. No payload was copied, mounted, installed, or executed.\n", value.ID, value.Status, value.ReviewedBy, reviewedAt, value.ReviewReason)
	return 0
}
