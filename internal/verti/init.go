package verti

import (
	"errors"
	"fmt"
	"os"

	"github.com/google/uuid"

	verticonfig "verti/internal/config"
	"verti/internal/editor"
	"verti/internal/gitrepo"
)

const (
	configPath  = ".git/verti.toml"
	hookPath    = ".git/hooks/reference-transaction"
	excludePath = ".git/info/exclude"
)

func Init(exePath string) error {
	if err := gitrepo.EnsureGitDir(); err != nil {
		return errors.New("not a git repository")
	}

	if _, err := os.Stat(configPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to check config: %v", err)
		}
		if err := verticonfig.WriteConfig(configPath, verticonfig.Config{
			RepoID:    uuid.NewString(),
			Artifacts: []string{},
		}); err != nil {
			return fmt.Errorf("failed to write config: %v", err)
		}
	}

	if err := gitrepo.WriteReferenceTransactionHook(hookPath, exePath); err != nil {
		return fmt.Errorf("failed to write hook: %v", err)
	}

	if err := editor.Open(configPath); err != nil {
		return fmt.Errorf("failed to open editor: %v", err)
	}

	cfg, err := verticonfig.ReadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	if err := gitrepo.ExcludeArtifacts(excludePath, cfg.Artifacts); err != nil {
		return fmt.Errorf("failed to update exclude: %v", err)
	}

	return nil
}
