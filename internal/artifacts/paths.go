/**
This file primarily deals with validating the list of artifact paths.
Absolute or outside of repo is invalid and fails fast.
For the valid ones, it build an sbolute path and checks if they
actually exist - define missing or present status.
*/

package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ArtifactStatusPresent = "present"
	ArtifactStatusMissing = "missing"
	ArtifactStatusSkipped = "skipped"
)

// ConfiguredPath represents one configured artifact path after normalization.
type ConfiguredPath struct {
	// Path is the normalized repo-relative path.
	Path string
	// AbsPath is the absolute path under repo root.
	AbsPath string
	// Status is present or missing for initial manifest planning.
	Status string
}

// NormalizeConfiguredPaths validates and normalizes configured artifact paths.
// It rejects absolute or escaping paths and marks absent entries as missing.
func NormalizeConfiguredPaths(repoRoot string, configured []string) ([]ConfiguredPath, error) {
	result := make([]ConfiguredPath, 0, len(configured))

	for _, raw := range configured {
		p := strings.TrimSpace(raw)
		if p == "" {
			return nil, fmt.Errorf("artifact path cannot be empty")
		}
		if filepath.IsAbs(p) {
			return nil, fmt.Errorf("artifact path %q must be repo-relative, not absolute", raw)
		}

		normalized := filepath.Clean(p)
		if containsParentTraversal(normalized) {
			return nil, fmt.Errorf("artifact path %q is invalid: contains parent traversal '..' after normalization (%q)", raw, normalized)
		}

		absPath := filepath.Join(repoRoot, normalized)
		status, err := artifactStatus(absPath)
		if err != nil {
			return nil, err
		}

		result = append(result, ConfiguredPath{
			Path:    normalized,
			AbsPath: absPath,
			Status:  status,
		})
	}

	return result, nil
}

func containsParentTraversal(p string) bool {
	parts := strings.Split(p, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}

func artifactStatus(absPath string) (string, error) {
	_, err := os.Lstat(absPath)
	if err == nil {
		return ArtifactStatusPresent, nil
	}
	if os.IsNotExist(err) {
		return ArtifactStatusMissing, nil
	}
	return "", fmt.Errorf("stat artifact %q: %w", absPath, err)
}
