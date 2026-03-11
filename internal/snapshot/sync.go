package snapshot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	verticonfig "verti/internal/config"
	"verti/internal/gitrepo"
)

const storageSubdir = ".verti/repos"

func Sync(cfg verticonfig.Config) error {
	artifacts, err := normalizeArtifacts(cfg.Artifacts)
	if err != nil {
		return err
	}
	if len(artifacts) == 0 {
		fmt.Println("no artifacts configured")
		return nil
	}

	head, err := gitrepo.Head()
	if err != nil {
		return fmt.Errorf("failed to get head: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to resolve home: %v", err)
	}

	snapshotDir := filepath.Join(home, storageSubdir, cfg.RepoID, head)
	info, err := os.Stat(snapshotDir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("failed to check snapshot: %s is not a directory", snapshotDir)
		}
		if err := restoreArtifacts(snapshotDir, artifacts); err != nil {
			return err
		}
		fmt.Printf("restore %s\n", head)
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check snapshot: %v", err)
	}

	repoStoreDir := filepath.Dir(snapshotDir)
	if err := os.MkdirAll(repoStoreDir, 0o755); err != nil {
		return fmt.Errorf("failed to create repo store: %v", err)
	}

	tempDir, err := os.MkdirTemp(repoStoreDir, head+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create snapshot staging dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	if err := snapshotArtifacts(tempDir, artifacts); err != nil {
		return err
	}
	if err := os.Rename(tempDir, snapshotDir); err != nil {
		return fmt.Errorf("failed to finalize snapshot: %v", err)
	}

	fmt.Printf("snapshot %s\n", head)
	return nil
}

func snapshotArtifacts(snapshotDir string, artifacts []string) error {
	for _, artifact := range artifacts {
		info, err := os.Stat(artifact)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("artifact not found: %s", artifact)
			}
			return fmt.Errorf("failed to check artifact %s: %v", artifact, err)
		}
		if info.IsDir() {
			return fmt.Errorf("artifact is a directory: %s", artifact)
		}
	}

	for _, artifact := range artifacts {
		if err := copyFile(artifact, filepath.Join(snapshotDir, artifact)); err != nil {
			return fmt.Errorf("failed to write snapshot for %s: %v", artifact, err)
		}
	}
	return nil
}

func restoreArtifacts(snapshotDir string, artifacts []string) error {
	for _, artifact := range artifacts {
		snapshotPath := filepath.Join(snapshotDir, artifact)
		info, err := os.Stat(snapshotPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("snapshot missing for artifact: %s", artifact)
			}
			return fmt.Errorf("failed to check snapshot for %s: %v", artifact, err)
		}
		if info.IsDir() {
			return fmt.Errorf("snapshot for artifact is a directory: %s", artifact)
		}
	}

	for _, artifact := range artifacts {
		snapshotPath := filepath.Join(snapshotDir, artifact)
		if err := copyFile(snapshotPath, artifact); err != nil {
			return fmt.Errorf("failed to restore snapshot for %s: %v", artifact, err)
		}
	}
	return nil
}

func normalizeArtifacts(artifacts []string) ([]string, error) {
	normalized := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		cleaned, err := normalizeArtifactPath(artifact)
		if err != nil {
			return nil, fmt.Errorf("invalid artifact path %q: %v", artifact, err)
		}
		normalized = append(normalized, cleaned)
	}
	return normalized, nil
}

func normalizeArtifactPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}
	if filepath.IsAbs(path) {
		return "", errors.New("must be relative")
	}

	cleaned := filepath.Clean(path)
	if cleaned == "." {
		return "", errors.New("must point to a file")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("must not escape repository")
	}

	return cleaned, nil
}

func copyFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, content, 0o644)
}
