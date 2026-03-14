package verti

import (
	"errors"
	"fmt"
	"os"

	"github.com/google/uuid"

	verticonfig "verti/internal/config"
	"verti/internal/editor"
	"verti/internal/gitrepo"
	"verti/internal/output"
)

const (
	configPath               = ".git/verti.toml"
	referenceTransactionPath = ".git/hooks/reference-transaction"
	postCheckoutHookPath     = ".git/hooks/post-checkout"
	excludePath              = ".git/info/exclude"
)

func Init(exePath string) error {
	if err := ensureInitialized(exePath); err != nil {
		return err
	}

	if err := editor.Open(configPath); err != nil {
		return fmt.Errorf("failed to open editor: %v", err)
	}

	return applyConfig()
}

func Add(exePath, artifactPath string) error {
	cleaned, err := verticonfig.NormalizeArtifactPath(artifactPath)
	if err != nil {
		return fmt.Errorf("invalid artifact path %q: %v", artifactPath, err)
	}

	if err := ensureInitialized(exePath); err != nil {
		return err
	}

	cfg, err := readConfig()
	if err != nil {
		return err
	}

	if containsString(cfg.Artifacts, cleaned) {
		if err := applyConfig(); err != nil {
			return err
		}
		output.Printf("Artifact already added: %s\n", cleaned)
		return nil
	}

	cfg.Artifacts = append(cfg.Artifacts, cleaned)
	if err := verticonfig.WriteConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to write config: %v", err)
	}

	if err := applyConfig(); err != nil {
		return err
	}

	output.Printf("Added artifact: %s\n", cleaned)
	return nil
}

func ensureInitialized(exePath string) error {
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

	if err := gitrepo.WriteReferenceTransactionHook(referenceTransactionPath, exePath); err != nil {
		return fmt.Errorf("failed to write reference-transaction hook: %v", err)
	}

	if err := gitrepo.WritePostCheckoutHook(postCheckoutHookPath, exePath); err != nil {
		return fmt.Errorf("failed to write post-checkout hook: %v", err)
	}

	return nil
}

func applyConfig() error {
	cfg, err := readConfig()
	if err != nil {
		return err
	}

	if err := gitrepo.ExcludeArtifacts(excludePath, cfg.Artifacts); err != nil {
		return fmt.Errorf("failed to update exclude: %v", err)
	}

	return nil
}

func readConfig() (verticonfig.Config, error) {
	cfg, err := verticonfig.ReadConfig(configPath)
	if err != nil {
		return verticonfig.Config{}, fmt.Errorf("failed to read config: %v", err)
	}
	return cfg, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
