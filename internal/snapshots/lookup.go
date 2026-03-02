package snapshots

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindSnapshot resolves <scopeDir>/snapshots/<sha> and reports whether it exists.
func FindSnapshot(scopeDir, sha string) (string, bool, error) {
	return findInCollection(scopeDir, "snapshots", sha)
}

// FindOrphanSnapshot resolves <scopeDir>/orphans/<orphanID> and reports whether it exists.
func FindOrphanSnapshot(scopeDir, orphanID string) (string, bool, error) {
	return findInCollection(scopeDir, "orphans", orphanID)
}

func findInCollection(scopeDir, collection, id string) (string, bool, error) {
	path := filepath.Join(scopeDir, collection, id)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, false, nil
		}
		return "", false, fmt.Errorf("stat path %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("path %q exists but is not a directory", path)
	}

	return path, true, nil
}
