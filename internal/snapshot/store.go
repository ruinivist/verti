package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type store struct {
	root string
}

func newStore(home, repoID string) store {
	return store{root: filepath.Join(home, storageSubdir, repoID)}
}

func (s store) ensureDirs() error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("failed to create repo store: %v", err)
	}
	if err := os.MkdirAll(s.blobsPath(), 0o755); err != nil {
		return fmt.Errorf("failed to create blob store: %v", err)
	}
	if err := os.MkdirAll(s.manifestsPath(), 0o755); err != nil {
		return fmt.Errorf("failed to create manifest store: %v", err)
	}
	if err := os.MkdirAll(s.orphansPath(), 0o755); err != nil {
		return fmt.Errorf("failed to create orphan store: %v", err)
	}

	return nil
}

func (s store) blobsPath() string {
	return filepath.Join(s.root, "blobs")
}

func (s store) manifestsPath() string {
	return filepath.Join(s.root, "manifests")
}

func (s store) orphansPath() string {
	return filepath.Join(s.root, "orphans")
}

func (s store) blobPath(hash string) string {
	return filepath.Join(s.blobsPath(), hash)
}

func (s store) manifestPath(commit string) string {
	return filepath.Join(s.manifestsPath(), commit+".json")
}

func (s store) orphanManifestPath(id string) string {
	return filepath.Join(s.orphansPath(), id+".json")
}

func (s store) orphanManifestPaths() ([]string, error) {
	entries, err := os.ReadDir(s.orphansPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read orphan store: %v", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		paths = append(paths, filepath.Join(s.orphansPath(), entry.Name()))
	}

	sort.Strings(paths)
	return paths, nil
}

func (s store) deleteOrphanManifest(id string) error {
	path := s.orphanManifestPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete orphan manifest: %v", err)
	}
	return nil
}

func orphanIDFromPath(path string) string {
	name := filepath.Base(path)
	return strings.TrimSuffix(name, filepath.Ext(name))
}
