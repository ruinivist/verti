package hooks

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEffectiveHooksDirDefaultsToGitHooks(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)

	got, err := EffectiveHooksDir(repoDir)
	if err != nil {
		t.Fatalf("EffectiveHooksDir() error = %v", err)
	}

	want := filepath.Join(repoDir, ".git", "hooks")
	if got != want {
		t.Fatalf("EffectiveHooksDir() = %q, want %q", got, want)
	}
}

func TestEffectiveHooksDirUsesAbsoluteCoreHooksPath(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)
	absHooksDir := filepath.Join(t.TempDir(), "hooks-abs")
	runGit(t, repoDir, "config", "core.hooksPath", absHooksDir)

	got, err := EffectiveHooksDir(repoDir)
	if err != nil {
		t.Fatalf("EffectiveHooksDir() error = %v", err)
	}

	if got != absHooksDir {
		t.Fatalf("EffectiveHooksDir() = %q, want %q", got, absHooksDir)
	}
}

func TestEffectiveHooksDirResolvesRelativeCoreHooksPathFromRepoRoot(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)
	runGit(t, repoDir, "config", "core.hooksPath", ".githooks")

	got, err := EffectiveHooksDir(repoDir)
	if err != nil {
		t.Fatalf("EffectiveHooksDir() error = %v", err)
	}

	want := filepath.Join(repoDir, ".githooks")
	if got != want {
		t.Fatalf("EffectiveHooksDir() = %q, want %q", got, want)
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
