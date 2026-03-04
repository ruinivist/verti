package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"verti/internal/git"
)

// EffectiveHooksDir returns the hooks directory Git will use for this worktree.
func EffectiveHooksDir(workingDir string) (string, error) {
	// via core.hooksPath config of git
	hooksPath, err := coreHooksPath(workingDir)
	if err != nil {
		return "", err
	}

	if hooksPath == "" {
		// default path is .git/hooks
		gitDir, err := git.GitDir(workingDir)
		if err != nil {
			return "", fmt.Errorf("resolve git dir: %w", err)
		}
		return filepath.Join(gitDir, "hooks"), nil
	}

	// core hooks, relative/abs handling
	if filepath.IsAbs(hooksPath) {
		return filepath.Clean(hooksPath), nil
	}

	repoRoot, err := git.RepoRoot(workingDir)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}

	return filepath.Clean(filepath.Join(repoRoot, hooksPath)), nil
}

// can be relative or absolute path, both would need to be handled
func coreHooksPath(workingDir string) (string, error) {
	cmd := exec.Command("git", "config", "--get", "core.hooksPath")
	cmd.Dir = workingDir

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
		// Exit code 1 means value is unset.
		return "", nil
	}

	return "", fmt.Errorf("read core.hooksPath: %w: %s", err, strings.TrimSpace(stderr.String()))
}
