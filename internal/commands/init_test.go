package commands

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"verti/internal/config"
)

func TestRunInitCreatesConfigWithUUID(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)

	if err := RunInit(repoDir); err != nil {
		t.Fatalf("RunInit() error = %v", err)
	}

	configPath := filepath.Join(repoDir, ".git", "verti.toml")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.RepoID == "" {
		t.Fatalf("expected repo_id to be generated")
	}
	if _, err := uuid.Parse(cfg.RepoID); err != nil {
		t.Fatalf("repo_id %q is not valid UUID: %v", cfg.RepoID, err)
	}
}

func TestRunInitKeepsSameRepoIDOnRerun(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)

	if err := RunInit(repoDir); err != nil {
		t.Fatalf("RunInit(first) error = %v", err)
	}
	cfgPath := filepath.Join(repoDir, ".git", "verti.toml")
	firstCfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(first) error = %v", err)
	}

	if err := RunInit(repoDir); err != nil {
		t.Fatalf("RunInit(second) error = %v", err)
	}
	secondCfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(second) error = %v", err)
	}

	if secondCfg.RepoID != firstCfg.RepoID {
		t.Fatalf("repo_id changed across rerun: first=%q second=%q", firstCfg.RepoID, secondCfg.RepoID)
	}
}

func TestRunInitOutsideGitWorktreeReturnsClearError(t *testing.T) {
	nonRepoDir := t.TempDir()

	err := RunInit(nonRepoDir)
	if err == nil {
		t.Fatalf("RunInit() expected error outside git worktree")
	}
	if !strings.Contains(err.Error(), "inside a git worktree") {
		t.Fatalf("expected clear git worktree error, got %v", err)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found in PATH")
	}
}

func createGitRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	return repoDir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s failed: %v stderr=%q", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String())
}
