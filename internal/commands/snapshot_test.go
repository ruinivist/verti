package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"verti/internal/config"
)

func TestRunSnapshotWritesSnapshotForCurrentHEAD(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-1",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSnapshot(repoDir, &stderr); err != nil {
		t.Fatalf("runSnapshot() error = %v", err)
	}

	sha := runGit(t, repoDir, "rev-parse", "HEAD")
	snapshotDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "snapshots", sha)
	if _, err := os.Stat(filepath.Join(snapshotDir, "manifest.json")); err != nil {
		t.Fatalf("manifest.json not found in published snapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snapshotDir, "meta.json")); err != nil {
		t.Fatalf("meta.json not found in published snapshot: %v", err)
	}
}

func TestRunSnapshotIncludesBranchMetadataButKeysByCommitSHA(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-branch",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSnapshot(repoDir, &stderr); err != nil {
		t.Fatalf("runSnapshot() error = %v", err)
	}

	sha := runGit(t, repoDir, "rev-parse", "HEAD")
	branch := runGit(t, repoDir, "branch", "--show-current")
	snapshotDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "snapshots", sha)

	metaRaw, err := os.ReadFile(filepath.Join(snapshotDir, "meta.json"))
	if err != nil {
		t.Fatalf("read meta.json: %v", err)
	}

	var metaDoc map[string]any
	if err := json.Unmarshal(metaRaw, &metaDoc); err != nil {
		t.Fatalf("unmarshal meta.json: %v", err)
	}

	if metaDoc["branch"] != branch {
		t.Fatalf("meta.branch = %v, want %q", metaDoc["branch"], branch)
	}
	if metaDoc["commit_sha"] != sha {
		t.Fatalf("meta.commit_sha = %v, want %q", metaDoc["commit_sha"], sha)
	}

	branchKeyPath := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "snapshots", branch)
	if _, err := os.Stat(branchKeyPath); err == nil {
		t.Fatalf("unexpected branch-keyed snapshot path exists: %s", branchKeyPath)
	}
}

func TestRunSnapshotSurfacesNonFatalStoreErrorsAsWarnings(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-warn",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	origWriteObject := writeObjectFn
	writeObjectFn = func(_ string, _ []byte) (string, error) {
		return "", errors.New("injected object-write failure")
	}
	t.Cleanup(func() { writeObjectFn = origWriteObject })

	var stderr bytes.Buffer
	if err := runSnapshot(repoDir, &stderr); err != nil {
		t.Fatalf("runSnapshot() returned fatal error on store failure: %v", err)
	}

	warnings := stderr.String()
	if !strings.Contains(warnings, "warning:") {
		t.Fatalf("expected warning output, got: %q", warnings)
	}
	if !strings.Contains(warnings, "injected object-write failure") {
		t.Fatalf("expected surfaced store error in warning output, got: %q", warnings)
	}

	sha := runGit(t, repoDir, "rev-parse", "HEAD")
	snapshotDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "snapshots", sha)
	if _, err := os.Stat(filepath.Join(snapshotDir, "manifest.json")); err != nil {
		t.Fatalf("snapshot publish should still succeed despite non-fatal store errors: %v", err)
	}
}

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
