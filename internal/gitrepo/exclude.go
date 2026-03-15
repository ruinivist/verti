package gitrepo

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	managedExcludeStart = "# ===== verti start ====="
	managedExcludeEnd   = "# ===== verti end ====="
)

// managedExcludeBlockRE matches the full Verti block and captures only its inner lines.
var managedExcludeBlockRE = regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(managedExcludeStart) + `\n(.*?)^` + regexp.QuoteMeta(managedExcludeEnd) + `\n?`)

// ReadManagedExcludes reads `.git/info/exclude`, extracts Verti's block, and returns its non-empty lines.
func ReadManagedExcludes(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}

	match := managedExcludeBlockRE.FindStringSubmatch(string(content))
	if len(match) < 2 || match[1] == "" {
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimSuffix(match[1], "\n"), "\n")
	artifacts := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		artifacts = append(artifacts, line)
	}

	return artifacts, nil
}

// WriteManagedExcludes removes any existing Verti block, appends a freshly rendered block, and leaves other lines intact.
func WriteManagedExcludes(path string, artifacts []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		content = nil
	}

	out := managedExcludeBlockRE.ReplaceAllString(string(content), "")
	out = strings.TrimRight(out, "\n")
	if out != "" {
		out += "\n"
	}
	out += makeManagedExcludeBlock(artifacts)

	return os.WriteFile(path, []byte(out), 0o644)
}

// makeManagedExcludeBlock builds the marker lines and writes each artifact on its own line between them.
func makeManagedExcludeBlock(artifacts []string) string {
	var builder strings.Builder
	builder.WriteString(managedExcludeStart)
	builder.WriteByte('\n')
	for _, artifact := range artifacts {
		builder.WriteString(rootedExcludePattern(artifact))
		builder.WriteByte('\n')
	}
	builder.WriteString(managedExcludeEnd)
	builder.WriteByte('\n')
	return builder.String()
}

func rootedExcludePattern(artifact string) string {
	if artifact == "" {
		return "/"
	}
	if strings.HasPrefix(artifact, "/") {
		return artifact
	}
	return "/" + artifact
}
