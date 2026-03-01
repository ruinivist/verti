package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"verti/internal/config"
	"verti/internal/git"
	"verti/internal/hooks"
)

// RunInit creates or updates <common_git_dir>/verti.toml and ensures repo_id exists.
func RunInit(workingDir string) error {
	vertiBinPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve verti executable path: %w", err)
	}

	vertiBinPath, err = filepath.Abs(vertiBinPath)
	if err != nil {
		return fmt.Errorf("resolve absolute verti executable path: %w", err)
	}

	return runInit(workingDir, vertiBinPath)
}

func runInit(workingDir, vertiBinPath string) error {
	if _, err := git.RepoRoot(workingDir); err != nil {
		return fmt.Errorf("verti init must be run inside a git worktree: %w", err)
	}

	commonGitDir, err := git.CommonGitDir(workingDir)
	if err != nil {
		return fmt.Errorf("resolve common git dir: %w", err)
	}

	configPath := filepath.Join(commonGitDir, "verti.toml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.RepoID == "" {
		cfg.RepoID = uuid.NewString()
	}

	if err := config.Save(configPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	hooksDir, err := hooks.EffectiveHooksDir(workingDir)
	if err != nil {
		return fmt.Errorf("resolve effective hooks dir: %w", err)
	}

	hookNames := []string{
		hooks.PostCommitHook,
		hooks.PostCheckoutHook,
		hooks.PostMergeHook,
		hooks.PostRewriteHook,
	}

	for _, hookName := range hookNames {
		hookPath := filepath.Join(hooksDir, hookName)
		if _, err := hooks.InstallHookDispatcher(hookPath, hookName, vertiBinPath); err != nil {
			return fmt.Errorf("install dispatcher for %s: %w", hookName, err)
		}
	}

	return nil
}
