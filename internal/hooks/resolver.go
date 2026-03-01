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
	repoRoot, err := git.RepoRoot(workingDir)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}

	gitDir, err := git.GitDir(workingDir)
	if err != nil {
		return "", fmt.Errorf("resolve git dir: %w", err)
	}

	hooksPath, found, err := coreHooksPath(workingDir)
	if err != nil {
		return "", err
	}
	if !found || hooksPath == "" {
		return filepath.Join(gitDir, "hooks"), nil
	}

	if filepath.IsAbs(hooksPath) {
		return filepath.Clean(hooksPath), nil
	}

	return filepath.Clean(filepath.Join(repoRoot, hooksPath)), nil
}

func coreHooksPath(workingDir string) (string, bool, error) {
	cmd := exec.Command("git", "config", "--get", "core.hooksPath")
	cmd.Dir = workingDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return strings.TrimSpace(stdout.String()), true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		// Exit code 1 means value is unset.
		return "", false, nil
	}

	return "", false, fmt.Errorf("read core.hooksPath: %w: %s", err, strings.TrimSpace(stderr.String()))
}
