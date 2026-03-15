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

func Init(exePath string) error {
	paths, err := ensureInitialized(exePath)
	if err != nil {
		return err
	}

	return applyConfig(paths)
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

	paths, err := ensureInitialized(exePath)
	if err != nil {
		return err
	}

	cfg, err := readConfig(paths.Config)
	if err != nil {
		return err
	}

	if containsString(cfg.Artifacts, cleaned) {
		if err := applyConfig(paths); err != nil {
			return err
		}
		output.Printf("Artifact already added: %s\n", cleaned)
		return nil
	}

	cfg.Artifacts = append(cfg.Artifacts, cleaned)
	if err := verticonfig.WriteConfig(paths.Config, cfg); err != nil {
		return fmt.Errorf("failed to write config: %v", err)
	}

	if err := applyConfig(paths); err != nil {
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

	paths, err := ensureInitialized(exePath)
	if err != nil {
		return err
	}

	managed, err := gitrepo.ReadManagedExcludes(paths.Exclude)
	if err != nil {
		return fmt.Errorf("failed to read exclude: %v", err)
	}

	if !containsString(managed, rootedArtifactPath(cleaned)) {
		output.Printf("Path was not part of excludes; nothing to do: %s\n", cleaned)
		return nil
	}

	cfg, err := readConfig(paths.Config)
	if err != nil {
		return err
	}

	filtered, removed := removeString(cfg.Artifacts, cleaned)
	if removed {
		cfg.Artifacts = filtered
		if err := verticonfig.WriteConfig(paths.Config, cfg); err != nil {
			return fmt.Errorf("failed to write config: %v", err)
		}
	}

	if err := applyConfig(paths); err != nil {
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

func ensureInitialized(exePath string) (gitrepo.Paths, error) {
	if err := gitrepo.EnsureGitDir(); err != nil {
		return gitrepo.Paths{}, errors.New("not a git repository")
	}

	paths, err := gitrepo.ResolvePaths()
	if err != nil {
		return gitrepo.Paths{}, fmt.Errorf("failed to resolve git paths: %v", err)
	}

	if _, err := os.Stat(paths.Config); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return gitrepo.Paths{}, fmt.Errorf("failed to check config: %v", err)
		}
		if err := verticonfig.WriteConfig(paths.Config, verticonfig.Config{
			RepoID:    uuid.NewString(),
			Artifacts: []string{},
		}); err != nil {
			return gitrepo.Paths{}, fmt.Errorf("failed to write config: %v", err)
		}
	}

	if err := gitrepo.WriteReferenceTransactionHook(paths.ReferenceTransactionHook, exePath); err != nil {
		return gitrepo.Paths{}, fmt.Errorf("failed to write reference-transaction hook: %v", err)
	}

	if err := gitrepo.WritePostCheckoutHook(paths.PostCheckoutHook, exePath); err != nil {
		return gitrepo.Paths{}, fmt.Errorf("failed to write post-checkout hook: %v", err)
	}

	return paths, nil
}

func applyConfig(paths gitrepo.Paths) error {
	cfg, err := readConfig(paths.Config)
	if err != nil {
		return err
	}

	if err := gitrepo.WriteManagedExcludes(paths.Exclude, cfg.Artifacts); err != nil {
		return fmt.Errorf("failed to update exclude: %v", err)
	}

	return nil
}

func readConfig(path string) (verticonfig.Config, error) {
	cfg, err := verticonfig.ReadConfig(path)
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
