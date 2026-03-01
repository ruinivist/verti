package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildManifestEntriesDeterministicOrderForMixedFixture(t *testing.T) {
	repoRoot := t.TempDir()
	createMixedFixture(t, repoRoot)

	configured := []string{"progress.md", "md", "missing.md"}

	first, err := BuildManifestEntries(repoRoot, configured)
	if err != nil {
		t.Fatalf("BuildManifestEntries(first) error = %v", err)
	}
	second, err := BuildManifestEntries(repoRoot, configured)
	if err != nil {
		t.Fatalf("BuildManifestEntries(second) error = %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("manifest entries are not deterministic across runs")
	}

	wantOrder := []string{
		"md",
		"md/a.txt",
		"md/link-to-progress",
		"md/sub",
		"md/sub/m.txt",
		"md/z.txt",
		"missing.md",
		"progress.md",
	}
	if len(first) != len(wantOrder) {
		t.Fatalf("expected %d manifest entries, got %d", len(wantOrder), len(first))
	}
	for i, wantPath := range wantOrder {
		if first[i].Path != wantPath {
			t.Fatalf("manifest order mismatch at index %d: got %q want %q", i, first[i].Path, wantPath)
		}
	}
}

func TestBuildManifestEntriesIncludesRequiredFields(t *testing.T) {
	repoRoot := t.TempDir()
	createMixedFixture(t, repoRoot)

	entries, err := BuildManifestEntries(repoRoot, []string{"progress.md", "md", "missing.md"})
	if err != nil {
		t.Fatalf("BuildManifestEntries() error = %v", err)
	}

	byPath := make(map[string]ManifestEntry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}

	dirEntry := byPath["md"]
	if dirEntry.Kind != ArtifactKindDir || dirEntry.Status != ArtifactStatusPresent {
		t.Fatalf("unexpected dir entry: %#v", dirEntry)
	}
	if dirEntry.Mode == 0 {
		t.Fatalf("expected dir mode to be set")
	}

	fileData := []byte("progress-content\n")
	fileEntry := byPath["progress.md"]
	if fileEntry.Kind != ArtifactKindFile || fileEntry.Status != ArtifactStatusPresent {
		t.Fatalf("unexpected file entry: %#v", fileEntry)
	}
	if fileEntry.Mode == 0 {
		t.Fatalf("expected file mode to be set")
	}
	if fileEntry.Size != int64(len(fileData)) {
		t.Fatalf("unexpected file size: got %d want %d", fileEntry.Size, len(fileData))
	}
	wantHash := sha256Hex(fileData)
	if fileEntry.Hash != wantHash {
		t.Fatalf("unexpected file hash: got %q want %q", fileEntry.Hash, wantHash)
	}

	symlinkEntry := byPath["md/link-to-progress"]
	if symlinkEntry.Kind != ArtifactKindSymlink || symlinkEntry.Status != ArtifactStatusPresent {
		t.Fatalf("unexpected symlink entry: %#v", symlinkEntry)
	}
	if symlinkEntry.Mode == 0 {
		t.Fatalf("expected symlink mode to be set")
	}
	if symlinkEntry.LinkTarget != "../progress.md" {
		t.Fatalf("unexpected symlink target: got %q", symlinkEntry.LinkTarget)
	}

	missingEntry := byPath["missing.md"]
	if missingEntry.Kind != ArtifactKindMissing || missingEntry.Status != ArtifactStatusMissing {
		t.Fatalf("unexpected missing entry: %#v", missingEntry)
	}
	if missingEntry.Mode != 0 || missingEntry.Size != 0 || missingEntry.Hash != "" || missingEntry.LinkTarget != "" {
		t.Fatalf("expected zero-value fields for missing entry, got %#v", missingEntry)
	}
}

func createMixedFixture(t *testing.T, repoRoot string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(repoRoot, "md", "sub"), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "progress.md"), []byte("progress-content\n"), 0o644); err != nil {
		t.Fatalf("write progress.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "md", "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "md", "z.txt"), []byte("z\n"), 0o644); err != nil {
		t.Fatalf("write z.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "md", "sub", "m.txt"), []byte("m\n"), 0o644); err != nil {
		t.Fatalf("write m.txt: %v", err)
	}
	if err := os.Symlink("../progress.md", filepath.Join(repoRoot, "md", "link-to-progress")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
