package config

import (
	"errors"
	"fmt"
	"strings"
)

func NormalizeArtifactPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}

	if strings.HasPrefix(path, "/") {
		path = strings.TrimPrefix(path, "/")
		if strings.HasPrefix(path, "/") {
			return "", errors.New("must not start with multiple slashes")
		}
	}
	if path == "" {
		return "", errors.New("must point to a file or directory")
	}
	if strings.HasPrefix(path, "../") || path == ".." {
		return "", errors.New("must not escape repository")
	}
	if strings.Contains(path, "/../") || strings.HasSuffix(path, "/..") {
		return "", errors.New("must not escape repository")
	}
	if strings.Contains(path, "//") {
		return "", errors.New("must not contain empty path segments")
	}
	if strings.HasPrefix(path, "./") || path == "." {
		return "", errors.New("must be rooted at repository top")
	}
	if strings.Contains(path, "/./") || strings.HasSuffix(path, "/.") {
		return "", errors.New("must be rooted at repository top")
	}

	return path, nil
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
