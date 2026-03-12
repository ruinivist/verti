package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
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

	return nil
}

func (s store) blobsPath() string {
	return filepath.Join(s.root, "blobs")
}

func (s store) manifestsPath() string {
	return filepath.Join(s.root, "manifests")
}

func (s store) blobPath(hash string) string {
	return filepath.Join(s.blobsPath(), hash)
}

func (s store) manifestPath(commit string) string {
	return filepath.Join(s.manifestsPath(), commit+".json")
}
