package snapshots

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindSnapshot resolves <scopeDir>/snapshots/<sha> and reports whether it exists.
func FindSnapshot(scopeDir, sha string) (string, bool, error) {
	path := filepath.Join(scopeDir, "snapshots", sha)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, false, nil
		}
		return "", false, fmt.Errorf("stat snapshot path %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("snapshot path %q exists but is not a directory", path)
	}

	return path, true, nil
}
