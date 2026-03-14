package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func NormalizeArtifactPath(path string) (string, error) {
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

func NormalizeArtifactPaths(artifacts []string) ([]string, error) {
	normalized := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		cleaned, err := NormalizeArtifactPath(artifact)
		if err != nil {
			return nil, fmt.Errorf("invalid artifact path %q: %v", artifact, err)
		}
		normalized = append(normalized, cleaned)
	}
	return normalized, nil
}
