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

	ctx, err := LoadContext(workingDir, []ContextField{
		ContextFieldConfig,
	})
	if err != nil {
		return err
	}

	configPath := ctx.Config.Path
	cfg := ctx.Config.Value

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
		hooks.ReferenceTransactionHook,
	}

	for _, hookName := range hookNames {
		hookPath := filepath.Join(hooksDir, hookName)
		if _, err := hooks.InstallHookDispatcher(hookPath, hookName, vertiBinPath); err != nil {
			return fmt.Errorf("install dispatcher for %s: %w", hookName, err)
		}
	}

	legacyHookNames := []string{
		"post-commit",
		"post-checkout",
		"post-merge",
		"post-rewrite",
	}
	for _, hookName := range legacyHookNames {
		hookPath := filepath.Join(hooksDir, hookName)
		if err := hooks.RemoveVertiDispatcher(hookPath); err != nil {
			return fmt.Errorf("cleanup legacy dispatcher for %s: %w", hookName, err)
		}
	}

	return nil
}
