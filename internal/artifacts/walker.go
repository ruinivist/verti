package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const (
	ArtifactKindFile    = "file"
	ArtifactKindDir     = "dir"
	ArtifactKindSymlink = "symlink"
	ArtifactKindMissing = "missing"
)

// ManifestEntry is the normalized file record used by snapshot manifests.
type ManifestEntry struct {
	Path       string
	Kind       string
	Mode       uint32
	Size       int64
	Hash       string
	LinkTarget string
	Status     string
}

// BuildManifestEntries normalizes configured paths and walks them deterministically.
func BuildManifestEntries(repoRoot string, configured []string) ([]ManifestEntry, error) {
	normalized, err := NormalizeConfiguredPaths(repoRoot, configured)
	if err != nil {
		return nil, err
	}

	return BuildManifestEntriesFromNormalized(normalized)
}

// BuildManifestEntriesFromNormalized walks already-normalized configured paths.
func BuildManifestEntriesFromNormalized(normalized []ConfiguredPath) ([]ManifestEntry, error) {
	entriesByPath := make(map[string]ManifestEntry)

	for _, cfg := range normalized {
		if cfg.Status == ArtifactStatusMissing {
			entriesByPath[toSlashPath(cfg.Path)] = ManifestEntry{
				Path:   toSlashPath(cfg.Path),
				Kind:   ArtifactKindMissing,
				Status: ArtifactStatusMissing,
			}
			continue
		}

		info, err := os.Lstat(cfg.AbsPath)
		if err != nil {
			return nil, fmt.Errorf("stat artifact %q: %w", cfg.Path, err)
		}

		if info.Mode().IsDir() {
			if err := walkDirectory(cfg, entriesByPath); err != nil {
				return nil, err
			}
			continue
		}

		entry, err := manifestEntryForPath(cfg.Path, cfg.AbsPath)
		if err != nil {
			return nil, err
		}
		entriesByPath[entry.Path] = entry
	}

	entries := make([]ManifestEntry, 0, len(entriesByPath))
	for _, e := range entriesByPath {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func walkDirectory(cfg ConfiguredPath, entries map[string]ManifestEntry) error {
	err := filepath.WalkDir(cfg.AbsPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relativePath := cfg.Path
		if path != cfg.AbsPath {
			rel, err := filepath.Rel(cfg.AbsPath, path)
			if err != nil {
				return fmt.Errorf("resolve relative path under %q: %w", cfg.Path, err)
			}
			relativePath = filepath.Join(cfg.Path, rel)
		}

		entry, err := manifestEntryForPath(relativePath, path)
		if err != nil {
			return err
		}
		entries[entry.Path] = entry

		return nil
	})
	if err != nil {
		return fmt.Errorf("walk artifact dir %q: %w", cfg.Path, err)
	}
	return nil
}

func manifestEntryForPath(relativePath, absPath string) (ManifestEntry, error) {
	info, err := os.Lstat(absPath)
	if err != nil {
		return ManifestEntry{}, fmt.Errorf("lstat %q: %w", absPath, err)
	}

	entry := ManifestEntry{
		Path:   toSlashPath(relativePath),
		Mode:   uint32(info.Mode().Perm()),
		Status: ArtifactStatusPresent,
	}

	switch {
	case info.Mode().IsRegular():
		entry.Kind = ArtifactKindFile
		entry.Size = info.Size()

		hash, err := sha256File(absPath)
		if err != nil {
			return ManifestEntry{}, err
		}
		entry.Hash = hash
	case info.Mode().IsDir():
		entry.Kind = ArtifactKindDir
	case info.Mode()&os.ModeSymlink != 0:
		entry.Kind = ArtifactKindSymlink
		target, err := os.Readlink(absPath)
		if err != nil {
			return ManifestEntry{}, fmt.Errorf("read symlink %q: %w", absPath, err)
		}
		entry.LinkTarget = target
	default:
		// Treat other filesystem node types as regular present entries by mode only.
		entry.Kind = ArtifactKindFile
	}

	return entry, nil
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file for hash %q: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func toSlashPath(p string) string {
	return filepath.ToSlash(filepath.Clean(p))
}
