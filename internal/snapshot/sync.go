package snapshot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	verticonfig "verti/internal/config"
	"verti/internal/gitrepo"
	"verti/internal/output"
)

const storageSubdir = ".verti/repos"
const manifestVersion = 1

type manifest struct {
	Version   int               `json:"version"`
	CreatedAt string            `json:"created_at,omitempty"`
	Artifacts map[string]string `json:"artifacts"`
}

type restorePlan struct {
	targets          []restoreTarget
	orphanCandidates []orphanCandidate
}

type restoreTarget struct {
	path    string
	hash    string
	content []byte
}

type orphanCandidate struct {
	path    string
	hash    string
	content []byte
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
		if err := restoreArtifacts(store, storedManifest); err != nil {
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

	storedManifest, blobContents, warnings, err := snapshotArtifacts(artifacts)
	if err != nil {
		return err
	}
	printWarnings(warnings)
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

func snapshotArtifacts(artifacts []string) (manifest, map[string][]byte, []string, error) {
	expanded, warnings, err := expandArtifacts(artifacts)
	if err != nil {
		return manifest{}, nil, nil, err
	}

	storedManifest := manifest{
		Version:   manifestVersion,
		CreatedAt: manifestTimestamp(time.Now()),
		Artifacts: make(map[string]string, len(expanded)),
	}
	blobContents := make(map[string][]byte, len(expanded))
	for _, artifact := range expanded {
		content, err := os.ReadFile(artifact)
		if err != nil {
			return manifest{}, nil, nil, fmt.Errorf("failed to read artifact %s: %v", artifact, err)
		}

		hash := hashContent(content)
		storedManifest.Artifacts[artifact] = hash
		if _, ok := blobContents[hash]; !ok {
			blobContents[hash] = content
		}
	}

	return storedManifest, blobContents, warnings, nil
}

func restoreArtifacts(store store, storedManifest manifest) error {
	plan, err := buildRestorePlan(store, storedManifest)
	if err != nil {
		return err
	}
	return applyRestorePlan(plan)
}

func expandArtifacts(artifacts []string) ([]string, []string, error) {
	expanded := make([]string, 0, len(artifacts))
	warnings := make([]string, 0)
	for _, artifact := range artifacts {
		paths, artifactWarnings, err := expandArtifact(artifact)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, artifactWarnings...)
		expanded = append(expanded, paths...)
	}

	sort.Strings(expanded)
	return uniqueStrings(expanded), warnings, nil
}

func expandArtifact(artifact string) ([]string, []string, error) {
	info, err := os.Lstat(artifact)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("artifact not found: %s", artifact)
		}
		return nil, nil, fmt.Errorf("failed to check artifact %s: %v", artifact, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("artifact is a symlink: %s", artifact)
	}
	if info.IsDir() {
		return walkArtifactDir(artifact)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("artifact is not a regular file: %s", artifact)
	}

	return []string{artifact}, nil, nil
}

func walkArtifactDir(root string) ([]string, []string, error) {
	paths := make([]string, 0)
	warnings := make([]string, 0)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk artifact %s: %v", root, err)
		}
		if path == root {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			warnings = append(warnings, fmt.Sprintf("warning: skipped symlink artifact %s", path))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("artifact is not a regular file: %s", path)
		}

		paths = append(paths, filepath.Clean(path))
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	sort.Strings(paths)
	return paths, warnings, nil
}

func buildRestorePlan(store store, storedManifest manifest) (restorePlan, error) {
	targets := manifestRestoreTargets(storedManifest)
	plan := restorePlan{
		targets:          make([]restoreTarget, 0, len(targets)),
		orphanCandidates: make([]orphanCandidate, 0, len(targets)),
	}

	for _, target := range targets {
		if !isValidHash(target.hash) {
			return restorePlan{}, fmt.Errorf("invalid manifest hash for artifact: %s", target.path)
		}

		info, err := os.Stat(store.blobPath(target.hash))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return restorePlan{}, fmt.Errorf("blob missing for artifact: %s", target.path)
			}
			return restorePlan{}, fmt.Errorf("failed to check blob for %s: %v", target.path, err)
		}
		if info.IsDir() {
			return restorePlan{}, fmt.Errorf("blob for artifact is a directory: %s", target.path)
		}

		targetContent, err := os.ReadFile(store.blobPath(target.hash))
		if err != nil {
			return restorePlan{}, fmt.Errorf("failed to read blob for %s: %v", target.path, err)
		}
		target.content = targetContent
		plan.targets = append(plan.targets, target)

		currentContent, err := os.ReadFile(target.path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return restorePlan{}, fmt.Errorf("failed to read current artifact %s: %v", target.path, err)
		}
		if bytes.Equal(currentContent, targetContent) {
			continue
		}

		plan.orphanCandidates = append(plan.orphanCandidates, orphanCandidate{
			path:    target.path,
			hash:    hashContent(currentContent),
			content: currentContent,
		})
	}

	return plan, nil
}

func applyRestorePlan(plan restorePlan) error {
	for _, target := range plan.targets {
		if err := writeFile(target.path, target.content); err != nil {
			return fmt.Errorf("failed to restore artifact %s: %v", target.path, err)
		}
	}

	return nil
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
	if storedManifest.CreatedAt != "" {
		parsedCreatedAt, err := time.Parse(time.RFC3339, storedManifest.CreatedAt)
		if err != nil {
			return manifest{}, fmt.Errorf("invalid manifest created_at: %v", err)
		}
		storedManifest.CreatedAt = parsedCreatedAt.UTC().Format(time.RFC3339)
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

func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func printWarnings(warnings []string) {
	for _, warning := range warnings {
		output.Printf("%s\n", warning)
	}
}

func uniqueStrings(paths []string) []string {
	if len(paths) == 0 {
		return paths
	}

	out := paths[:1]
	for _, path := range paths[1:] {
		if path == out[len(out)-1] {
			continue
		}
		out = append(out, path)
	}

	return out
}

func manifestTimestamp(at time.Time) string {
	return at.UTC().Format(time.RFC3339)
}

func manifestRestoreTargets(storedManifest manifest) []restoreTarget {
	artifacts := make([]restoreTarget, 0, len(storedManifest.Artifacts))
	for path, hash := range storedManifest.Artifacts {
		artifacts = append(artifacts, restoreTarget{path: path, hash: hash})
	}

	// to make stable
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].path < artifacts[j].path
	})
	return artifacts
}
