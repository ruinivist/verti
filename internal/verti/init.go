package verti

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	verticonfig "verti/internal/config"
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

	return applyConfig()
}

func Add(exePath, artifactPath string) error {
	cleaned, err := normalizeArtifactPath(artifactPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(strings.TrimSuffix(cleaned, "/"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			output.Println("Warn: path doesn't exist but marking it as artifact for future.")
		} else {
			return fmt.Errorf("failed to check artifact %s: %v", cleaned, err)
		}
	} else if strings.HasSuffix(cleaned, "/") && !info.IsDir() {
		return fmt.Errorf("artifact is not a directory: %s", strings.TrimSuffix(cleaned, "/"))
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

func Remove(exePath, artifactPath string) error {
	cleaned, err := normalizeArtifactPath(artifactPath)
	if err != nil {
		return err
	}

	if err := ensureInitialized(exePath); err != nil {
		return err
	}

	managed, err := gitrepo.ReadManagedExcludes(excludePath)
	if err != nil {
		return fmt.Errorf("failed to read exclude: %v", err)
	}

	if !containsString(managed, rootedArtifactPath(cleaned)) {
		output.Printf("Path was not part of excludes; nothing to do: %s\n", cleaned)
		return nil
	}

	cfg, err := readConfig()
	if err != nil {
		return err
	}

	filtered, removed := removeString(cfg.Artifacts, cleaned)
	if removed {
		cfg.Artifacts = filtered
		if err := verticonfig.WriteConfig(configPath, cfg); err != nil {
			return fmt.Errorf("failed to write config: %v", err)
		}
	}

	if err := applyConfig(); err != nil {
		return err
	}

	output.Printf("Removed artifact: %s\n", cleaned)
	return nil
}

func normalizeArtifactPath(artifactPath string) (string, error) {
	cleaned, err := verticonfig.NormalizeArtifactPath(artifactPath)
	if err != nil {
		return "", fmt.Errorf("invalid artifact path %q: %v", artifactPath, err)
	}
	return cleaned, nil
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

	if err := gitrepo.WriteManagedExcludes(excludePath, cfg.Artifacts); err != nil {
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

func removeString(values []string, target string) ([]string, bool) {
	filtered := values[:0]
	removed := false
	for _, value := range values {
		if value == target {
			removed = true
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered, removed
}

func rootedArtifactPath(artifact string) string {
	return "/" + artifact
}
