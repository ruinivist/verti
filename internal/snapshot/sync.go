package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	verticonfig "verti/internal/config"
	"verti/internal/gitrepo"
	"verti/internal/output"
)

const storageSubdir = ".verti/repos"
const manifestVersion = 1

type manifest struct {
	Version   int               `json:"version"`
	Artifacts map[string]string `json:"artifacts"`
}

func Sync(cfg verticonfig.Config) error {
	artifacts, err := normalizeArtifacts(cfg.Artifacts)
	if err != nil {
		return err
	}
	if len(artifacts) == 0 {
		output.Println("no artifacts configured")
		return nil
	}

	head, err := gitrepo.Head()
	if err != nil {
		return fmt.Errorf("failed to get head: %v", err)
	}
	headDisplay, err := gitrepo.HeadDisplay()
	if err != nil {
		return fmt.Errorf("failed to get head display: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to resolve home: %v", err)
	}

	store := newStore(home, cfg.RepoID)
	manifestPath := store.manifestPath(head)
	info, err := os.Stat(manifestPath)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("failed to check manifest: %s is a directory", manifestPath)
		}

		storedManifest, err := loadManifest(manifestPath)
		if err != nil {
			return err
		}
		if err := restoreArtifacts(store, storedManifest, artifacts); err != nil {
			return err
		}

		output.Printf("Restored artifacts at %s\n", headDisplay)
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check manifest: %v", err)
	}

	if err := store.ensureDirs(); err != nil {
		return err
	}

	storedManifest, blobContents, err := snapshotArtifacts(artifacts)
	if err != nil {
		return err
	}
	for hash, content := range blobContents {
		if err := writeBlob(store, hash, content); err != nil {
			return err
		}
	}
	if err := writeManifest(manifestPath, storedManifest); err != nil {
		return err
	}

	output.Printf("Created artifacts for %s\n", headDisplay)
	return nil
}

func snapshotArtifacts(artifacts []string) (manifest, map[string][]byte, error) {
	if err := validateArtifactInputs(artifacts); err != nil {
		return manifest{}, nil, err
	}

	storedManifest := manifest{
		Version:   manifestVersion,
		Artifacts: make(map[string]string, len(artifacts)),
	}
	blobContents := make(map[string][]byte, len(artifacts))
	for _, artifact := range artifacts {
		content, err := os.ReadFile(artifact)
		if err != nil {
			return manifest{}, nil, fmt.Errorf("failed to read artifact %s: %v", artifact, err)
		}

		hash := hashContent(content)
		storedManifest.Artifacts[artifact] = hash
		if _, ok := blobContents[hash]; !ok {
			blobContents[hash] = content
		}
	}

	return storedManifest, blobContents, nil
}

func restoreArtifacts(store store, storedManifest manifest, artifacts []string) error {
	hashes, err := validateRestoreInputs(store, storedManifest, artifacts)
	if err != nil {
		return err
	}

	for _, artifact := range artifacts {
		hash := hashes[artifact]
		if err := copyFile(store.blobPath(hash), artifact); err != nil {
			return fmt.Errorf("failed to restore artifact %s: %v", artifact, err)
		}
	}

	return nil
}

func validateArtifactInputs(artifacts []string) error {
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

	return nil
}

func validateRestoreInputs(store store, storedManifest manifest, artifacts []string) (map[string]string, error) {
	hashes := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		hash, err := manifestHashForArtifact(storedManifest, artifact)
		if err != nil {
			return nil, err
		}

		info, err := os.Stat(store.blobPath(hash))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("blob missing for artifact: %s", artifact)
			}
			return nil, fmt.Errorf("failed to check blob for %s: %v", artifact, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("blob for artifact is a directory: %s", artifact)
		}

		hashes[artifact] = hash
	}

	return hashes, nil
}

func manifestHashForArtifact(storedManifest manifest, artifact string) (string, error) {
	hash, ok := storedManifest.Artifacts[artifact]
	if !ok {
		return "", fmt.Errorf("manifest missing artifact: %s", artifact)
	}
	if !isValidHash(hash) {
		return "", fmt.Errorf("invalid manifest hash for artifact: %s", artifact)
	}

	return hash, nil
}

func loadManifest(path string) (manifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, fmt.Errorf("failed to read manifest: %v", err)
	}

	var storedManifest manifest
	if err := json.Unmarshal(content, &storedManifest); err != nil {
		return manifest{}, fmt.Errorf("failed to decode manifest: %v", err)
	}
	if storedManifest.Version != manifestVersion {
		return manifest{}, fmt.Errorf("unsupported manifest version: %d", storedManifest.Version)
	}
	if storedManifest.Artifacts == nil {
		storedManifest.Artifacts = map[string]string{}
	}

	return storedManifest, nil
}

func writeManifest(path string, storedManifest manifest) error {
	content, err := json.MarshalIndent(storedManifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode manifest: %v", err)
	}
	content = append(content, '\n')

	if err := writeAtomic(path, filepath.Base(path)+".tmp-*", content); err != nil {
		return fmt.Errorf("failed to write manifest: %v", err)
	}

	return nil
}

func writeBlob(store store, hash string, content []byte) error {
	path := store.blobPath(hash)
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("failed to check blob: %s is a directory", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check blob: %v", err)
	}

	if err := writeAtomic(path, hash+".tmp-*", content); err != nil {
		return fmt.Errorf("failed to write blob %s: %v", hash, err)
	}

	return nil
}

func writeAtomic(path, pattern string, content []byte) error {
	tempFile, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return fmt.Errorf("create staging file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return fmt.Errorf("write staging file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close staging file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename staging file: %w", err)
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

func hashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func isValidHash(hash string) bool {
	if len(hash) != sha256.Size*2 {
		return false
	}

	decoded, err := hex.DecodeString(hash)
	return err == nil && len(decoded) == sha256.Size
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
