package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// RepoRoot returns the absolute path to the current worktree root.
func RepoRoot(dir string) (string, error) {
	return runGit(dir, "rev-parse", "--show-toplevel")
}

// GitDir returns the absolute git directory for the current worktree.
func GitDir(dir string) (string, error) {
	return runGit(dir, "rev-parse", "--absolute-git-dir")
}

// CommonGitDir returns the absolute common git directory shared by worktrees.
func CommonGitDir(dir string) (string, error) {
	return runGit(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
}

// HeadSHA returns the current HEAD commit SHA.
func HeadSHA(dir string) (string, error) {
	return runGit(dir, "rev-parse", "HEAD")
}

// CurrentBranch returns the current branch short name.
// In detached HEAD state, it returns an empty string and nil error.
func CurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "symbolic-ref", "--quiet", "--short", "HEAD")
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return strings.TrimSpace(stdout.String()), nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		// Exit code 1 here means detached HEAD.
		return "", nil
	}

	return "", fmt.Errorf("git symbolic-ref --quiet --short HEAD failed: %w: %s", err, strings.TrimSpace(stderr.String()))
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}
