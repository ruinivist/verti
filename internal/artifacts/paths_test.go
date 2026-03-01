package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeConfiguredPathsRejectsAbsolutePaths(t *testing.T) {
	repoRoot := t.TempDir()

	_, err := NormalizeConfiguredPaths(repoRoot, []string{"/tmp/outside"})
	if err == nil {
		t.Fatalf("NormalizeConfiguredPaths() expected absolute-path error")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path error message, got %v", err)
	}
}

func TestNormalizeConfiguredPathsRejectsParentTraversalAfterNormalization(t *testing.T) {
	repoRoot := t.TempDir()

	_, err := NormalizeConfiguredPaths(repoRoot, []string{"foo/../../outside"})
	if err == nil {
		t.Fatalf("NormalizeConfiguredPaths() expected parent-traversal error")
	}
	if !strings.Contains(err.Error(), "..") {
		t.Fatalf("expected .. traversal error message, got %v", err)
	}
}

func TestNormalizeConfiguredPathsMarksMissingArtifactsForManifest(t *testing.T) {
	repoRoot := t.TempDir()

	existing := filepath.Join(repoRoot, "md")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("mkdir existing artifact: %v", err)
	}

	got, err := NormalizeConfiguredPaths(repoRoot, []string{"md", "progress.md"})
	if err != nil {
		t.Fatalf("NormalizeConfiguredPaths() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 artifact entries, got %d", len(got))
	}

	if got[0].Path != "md" || got[0].Status != ArtifactStatusPresent {
		t.Fatalf("unexpected first entry: %#v", got[0])
	}

	if got[1].Path != "progress.md" || got[1].Status != ArtifactStatusMissing {
		t.Fatalf("expected missing second entry, got %#v", got[1])
	}
}
