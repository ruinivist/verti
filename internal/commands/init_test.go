package commands

import (
	"bytes"
	"os"
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

func TestRunInitInstallsAllRequiredDispatchers(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)

	if err := runInit(repoDir, "/abs/path/to/verti"); err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	for _, hookName := range []string{"post-commit", "post-checkout", "post-merge", "post-rewrite"} {
		hookPath := filepath.Join(repoDir, ".git", "hooks", hookName)

		info, err := os.Stat(hookPath)
		if err != nil {
			t.Fatalf("stat hook %q: %v", hookPath, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("hook %q is not executable (mode=%o)", hookPath, info.Mode().Perm())
		}

		content := mustReadFile(t, hookPath)
		if !strings.Contains(content, "# verti-dispatcher") {
			t.Fatalf("hook %q missing dispatcher marker:\n%s", hookPath, content)
		}
		if !strings.Contains(content, "VERTI_BIN=\"/abs/path/to/verti\"") {
			t.Fatalf("hook %q missing embedded verti binary path:\n%s", hookPath, content)
		}
	}
}

func TestRunInitIsIdempotentForInstalledDispatchers(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)

	if err := runInit(repoDir, "/abs/path/to/verti"); err != nil {
		t.Fatalf("runInit(first) error = %v", err)
	}

	firstContents := make(map[string]string, 4)
	for _, hookName := range []string{"post-commit", "post-checkout", "post-merge", "post-rewrite"} {
		hookPath := filepath.Join(repoDir, ".git", "hooks", hookName)
		firstContents[hookName] = mustReadFile(t, hookPath)
	}

	if err := runInit(repoDir, "/abs/path/to/verti"); err != nil {
		t.Fatalf("runInit(second) error = %v", err)
	}

	for hookName, first := range firstContents {
		hookPath := filepath.Join(repoDir, ".git", "hooks", hookName)
		second := mustReadFile(t, hookPath)
		if second != first {
			t.Fatalf("hook %q changed across idempotent rerun", hookName)
		}

		backupPattern := hookPath + ".verti.backup*"
		backupMatches, err := filepath.Glob(backupPattern)
		if err != nil {
			t.Fatalf("glob %q: %v", backupPattern, err)
		}
		if len(backupMatches) != 0 {
			t.Fatalf("expected no backup files for clean install rerun; got %v", backupMatches)
		}
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(data)
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
