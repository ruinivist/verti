package git

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoPathsInMainAndLinkedWorktree(t *testing.T) {
	requireGit(t)

	repoDir, worktreeDir := createRepoWithLinkedWorktree(t)

	mainRepoRoot, err := RepoRoot(repoDir)
	if err != nil {
		t.Fatalf("RepoRoot(main) error: %v", err)
	}
	if mainRepoRoot != repoDir {
		t.Fatalf("RepoRoot(main) = %q, want %q", mainRepoRoot, repoDir)
	}

	mainGitDir, err := GitDir(repoDir)
	if err != nil {
		t.Fatalf("GitDir(main) error: %v", err)
	}
	wantMainGitDir := filepath.Join(repoDir, ".git")
	if mainGitDir != wantMainGitDir {
		t.Fatalf("GitDir(main) = %q, want %q", mainGitDir, wantMainGitDir)
	}

	mainCommonGitDir, err := CommonGitDir(repoDir)
	if err != nil {
		t.Fatalf("CommonGitDir(main) error: %v", err)
	}
	if mainCommonGitDir != wantMainGitDir {
		t.Fatalf("CommonGitDir(main) = %q, want %q", mainCommonGitDir, wantMainGitDir)
	}

	worktreeRepoRoot, err := RepoRoot(worktreeDir)
	if err != nil {
		t.Fatalf("RepoRoot(worktree) error: %v", err)
	}
	if worktreeRepoRoot != worktreeDir {
		t.Fatalf("RepoRoot(worktree) = %q, want %q", worktreeRepoRoot, worktreeDir)
	}

	worktreeGitDir, err := GitDir(worktreeDir)
	if err != nil {
		t.Fatalf("GitDir(worktree) error: %v", err)
	}
	wantWorktreeGitDir := filepath.Join(repoDir, ".git", "worktrees", filepath.Base(worktreeDir))
	if worktreeGitDir != wantWorktreeGitDir {
		t.Fatalf("GitDir(worktree) = %q, want %q", worktreeGitDir, wantWorktreeGitDir)
	}

	worktreeCommonGitDir, err := CommonGitDir(worktreeDir)
	if err != nil {
		t.Fatalf("CommonGitDir(worktree) error: %v", err)
	}
	if worktreeCommonGitDir != wantMainGitDir {
		t.Fatalf("CommonGitDir(worktree) = %q, want %q", worktreeCommonGitDir, wantMainGitDir)
	}
}

func TestHeadSHAAndCurrentBranchAttachedAndDetached(t *testing.T) {
	requireGit(t)

	repoDir := createRepoWithCommit(t)

	wantHeadSHA := runGitCmd(t, repoDir, "rev-parse", "HEAD")
	gotHeadSHA, err := HeadSHA(repoDir)
	if err != nil {
		t.Fatalf("HeadSHA(attached) error: %v", err)
	}
	if gotHeadSHA != wantHeadSHA {
		t.Fatalf("HeadSHA(attached) = %q, want %q", gotHeadSHA, wantHeadSHA)
	}

	wantBranch := runGitCmd(t, repoDir, "branch", "--show-current")
	gotBranch, err := CurrentBranch(repoDir)
	if err != nil {
		t.Fatalf("CurrentBranch(attached) error: %v", err)
	}
	if gotBranch != wantBranch {
		t.Fatalf("CurrentBranch(attached) = %q, want %q", gotBranch, wantBranch)
	}

	runGitCmd(t, repoDir, "checkout", "--detach", "HEAD")

	gotDetachedBranch, err := CurrentBranch(repoDir)
	if err != nil {
		t.Fatalf("CurrentBranch(detached) error: %v", err)
	}
	if gotDetachedBranch != "" {
		t.Fatalf("CurrentBranch(detached) = %q, want empty string", gotDetachedBranch)
	}

	gotDetachedHeadSHA, err := HeadSHA(repoDir)
	if err != nil {
		t.Fatalf("HeadSHA(detached) error: %v", err)
	}
	if gotDetachedHeadSHA != wantHeadSHA {
		t.Fatalf("HeadSHA(detached) = %q, want %q", gotDetachedHeadSHA, wantHeadSHA)
	}
}

func createRepoWithLinkedWorktree(t *testing.T) (string, string) {
	t.Helper()

	repoDir := createRepoWithCommit(t)
	runGitCmd(t, repoDir, "branch", "worktree-branch")

	worktreeDir := filepath.Join(filepath.Dir(repoDir), "repo-worktree")
	runGitCmd(t, repoDir, "worktree", "add", worktreeDir, "worktree-branch")

	return repoDir, worktreeDir
}

func createRepoWithCommit(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	runGitCmd(t, repoDir, "init")
	runGitCmd(t, repoDir, "config", "user.email", "test@example.com")
	runGitCmd(t, repoDir, "config", "user.name", "Test User")

	filePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(filePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	runGitCmd(t, repoDir, "add", "README.md")
	runGitCmd(t, repoDir, "commit", "-m", "initial")

	return repoDir
}

func requireGit(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found in PATH")
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s failed: %v, stderr=%q", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String())
}
