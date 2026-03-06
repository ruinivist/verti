package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"verti/internal/config"
)

func createGitRepoWithArtifacts(t *testing.T) string {
	t.Helper()

	repoDir := createGitRepo(t)
	if err := os.MkdirAll(filepath.Join(repoDir, "md"), 0o755); err != nil {
		t.Fatalf("mkdir md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "md", "note.md"), []byte("note\n"), 0o644); err != nil {
		t.Fatalf("write md/note.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("progress\n"), 0o644); err != nil {
		t.Fatalf("write progress.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatalf("write tracked.txt: %v", err)
	}
	runGit(t, repoDir, "add", "tracked.txt")
	runGit(t, repoDir, "commit", "-m", "tracked commit")

	return repoDir
}

func writeRepoConfig(t *testing.T, repoDir string, cfg config.Config) {
	t.Helper()
	path := filepath.Join(repoDir, ".git", "verti.toml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save config at %q: %v", path, err)
	}
}

func readManifestEntries(t *testing.T, manifestPath string) map[string]struct {
	Path   string `json:"path"`
	Hash   string `json:"hash"`
	Status string `json:"status"`
} {
	t.Helper()

	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest %q: %v", manifestPath, err)
	}

	var doc struct {
		Entries []struct {
			Path   string `json:"path"`
			Hash   string `json:"hash"`
			Status string `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal manifest %q: %v", manifestPath, err)
	}

	byPath := make(map[string]struct {
		Path   string `json:"path"`
		Hash   string `json:"hash"`
		Status string `json:"status"`
	}, len(doc.Entries))
	for _, e := range doc.Entries {
		byPath[e.Path] = e
	}
	return byPath
}
