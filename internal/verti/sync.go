package verti

import (
	"errors"
	"fmt"

	verticonfig "verti/internal/config"
	"verti/internal/gitrepo"
	"verti/internal/snapshot"
)

func Sync() error {
	if err := gitrepo.EnsureGitDir(); err != nil {
		return errors.New("not a git repository")
	}

	cfg, err := verticonfig.ReadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	return snapshot.Sync(cfg)
}
