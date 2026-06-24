package process

import (
	"context"
	"testing"
)

func TestLoadReferencedLucyManifestParsesPackages(t *testing.T) {
	referenced, err := loadReferencedLucyManifest(context.Background(), ".", "manifests/gtmc-1.17-base.lucy.yaml")
	if err != nil {
		t.Fatalf("load referenced manifest: %v", err)
	}
	if len(referenced.manifest.Packages) != 2 {
		t.Fatalf("manifest packages = %d, want 2: %#v", len(referenced.manifest.Packages), referenced.manifest.Packages)
	}
	if len(referenced.packages) != 2 {
		t.Fatalf("package refs = %d, want 2: %#v", len(referenced.packages), referenced.packages)
	}
}
