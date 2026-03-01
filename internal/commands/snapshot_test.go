package commands

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

func TestRunSnapshotSkipsFilesAboveMaxFileSizeAndDoesNotWriteObject(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	largeContent := bytes.Repeat([]byte("x"), 2*1024*1024) // 2 MB
	if err := os.WriteFile(filepath.Join(repoDir, "big.bin"), largeContent, 0o644); err != nil {
		t.Fatalf("write big.bin: %v", err)
	}

	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-large",
		Enabled:       true,
		Artifacts:     []string{"big.bin"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: 1,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSnapshot(repoDir, &stderr); err != nil {
		t.Fatalf("runSnapshot() error = %v", err)
	}

	sha := runGit(t, repoDir, "rev-parse", "HEAD")
	snapshotDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "snapshots", sha)
	manifestEntries := readManifestEntries(t, filepath.Join(snapshotDir, "manifest.json"))

	entry, ok := manifestEntries["big.bin"]
	if !ok {
		t.Fatalf("manifest missing big.bin entry")
	}
	if entry.Status != "skipped" {
		t.Fatalf("big.bin status = %q, want %q", entry.Status, "skipped")
	}
	if entry.Hash != "" {
		t.Fatalf("expected skipped entry hash to be empty, got %q", entry.Hash)
	}

	sum := sha256.Sum256(largeContent)
	objectPath := filepath.Join(storeRoot, "repos", cfg.RepoID, "objects", fmt.Sprintf("%x", sum))
	if _, err := os.Stat(objectPath); !os.IsNotExist(err) {
		t.Fatalf("expected no object file for skipped artifact, stat err=%v", err)
	}

	if !strings.Contains(stderr.String(), "warning:") || !strings.Contains(stderr.String(), "max_file_size_mb") {
		t.Fatalf("expected max-file-size warning, got %q", stderr.String())
	}
}

func TestRunSnapshotStoresFilesBelowMaxFileSizeNormally(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	smallContent := []byte("small\n")
	if err := os.WriteFile(filepath.Join(repoDir, "small.bin"), smallContent, 0o644); err != nil {
		t.Fatalf("write small.bin: %v", err)
	}

	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-small",
		Enabled:       true,
		Artifacts:     []string{"small.bin"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: 1,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSnapshot(repoDir, &stderr); err != nil {
		t.Fatalf("runSnapshot() error = %v", err)
	}

	sha := runGit(t, repoDir, "rev-parse", "HEAD")
	snapshotDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "snapshots", sha)
	manifestEntries := readManifestEntries(t, filepath.Join(snapshotDir, "manifest.json"))

	entry, ok := manifestEntries["small.bin"]
	if !ok {
		t.Fatalf("manifest missing small.bin entry")
	}
	if entry.Status != "present" {
		t.Fatalf("small.bin status = %q, want %q", entry.Status, "present")
	}
	if entry.Hash == "" {
		t.Fatalf("expected non-empty hash for small.bin")
	}

	objectPath := filepath.Join(storeRoot, "repos", cfg.RepoID, "objects", entry.Hash)
	if _, err := os.Stat(objectPath); err != nil {
		t.Fatalf("expected object file for non-skipped artifact: %v", err)
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
