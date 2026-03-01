package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"verti/internal/cli"
	"verti/internal/config"
)

func TestRunRestoreMissingSnapshotNoOpLeavesFilesUnchanged(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-noop",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	before, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read pre-restore progress.md: %v", err)
	}

	var stderr bytes.Buffer
	if err := runRestore(repoDir, []string{"deadbeef"}, &stderr); err != nil {
		t.Fatalf("runRestore() error = %v", err)
	}

	after, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read post-restore progress.md: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("restore no-op changed artifact contents")
	}

	if _, err := os.Stat(storeRoot); !os.IsNotExist(err) {
		t.Fatalf("restore no-op should not create store filesystem changes, stat err=%v", err)
	}
}

func TestRunRestoreArgumentContract(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-args",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	err := runRestore(repoDir, []string{}, &stderr)
	var usageErr *cli.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected usage error for missing target, got %v", err)
	}

	err = runRestore(repoDir, []string{"--orphan"}, &stderr)
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected usage error for --orphan without id, got %v", err)
	}

	if err := runRestore(repoDir, []string{"--orphan", "orphan-1"}, &stderr); err != nil {
		t.Fatalf("expected --orphan mode to satisfy argument contract, got %v", err)
	}

	if err := runRestore(repoDir, []string{"cafebabe"}, &stderr); err != nil {
		t.Fatalf("expected sha target to satisfy argument contract, got %v", err)
	}

	if strings.Contains(stderr.String(), "panic") {
		t.Fatalf("unexpected stderr output: %q", stderr.String())
	}
}
