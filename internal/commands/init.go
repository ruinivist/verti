package commands

import (
	"fmt"
	"path/filepath"

	"github.com/google/uuid"

	"verti/internal/config"
	"verti/internal/git"
)

// RunInit creates or updates <common_git_dir>/verti.toml and ensures repo_id exists.
func RunInit(workingDir string) error {
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

	return nil
}
