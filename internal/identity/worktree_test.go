package identity

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWorktreeIdentityPrimaryWorktreeIsMain(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)

	id, err := ResolveWorktreeIdentity(repoDir)
	if err != nil {
		t.Fatalf("ResolveWorktreeIdentity() error = %v", err)
	}

	if id.WorktreeID != "main" {
		t.Fatalf("WorktreeID = %q, want %q", id.WorktreeID, "main")
	}

	if id.WorktreePathFingerprint == "" {
		t.Fatalf("expected non-empty worktree path fingerprint")
	}
}

func TestResolveWorktreeIdentityLinkedWorktreeUsesGitDirBasename(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithCommit(t)
	runGit(t, repoDir, "branch", "feature")

	worktreeDir := filepath.Join(filepath.Dir(repoDir), "feature-wt")
	runGit(t, repoDir, "worktree", "add", worktreeDir, "feature")

	id, err := ResolveWorktreeIdentity(worktreeDir)
	if err != nil {
		t.Fatalf("ResolveWorktreeIdentity() error = %v", err)
	}

	if id.WorktreeID != filepath.Base(worktreeDir) {
		t.Fatalf("WorktreeID = %q, want %q", id.WorktreeID, filepath.Base(worktreeDir))
	}
}

func TestResolveWorktreeIdentityStoresPathFingerprint(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)

	id, err := ResolveWorktreeIdentity(repoDir)
	if err != nil {
		t.Fatalf("ResolveWorktreeIdentity() error = %v", err)
	}

	want, err := PathFingerprint(repoDir)
	if err != nil {
		t.Fatalf("PathFingerprint() error = %v", err)
	}
	if id.WorktreePathFingerprint != want {
		t.Fatalf("fingerprint mismatch: got %q want %q", id.WorktreePathFingerprint, want)
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

func createGitRepoWithCommit(t *testing.T) string {
	t.Helper()

	repoDir := createGitRepo(t)
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "initial")

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
