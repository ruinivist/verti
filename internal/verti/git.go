package verti

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func ensureGitDir() error {
	info, err := os.Stat(".git")
	if err != nil || !info.IsDir() {
		return errors.New("missing .git")
	}
	return nil
}

func gitExcludeArtifacts(path string, artifacts []string) error {
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

	lines := strings.Split(string(content), "\n")
	existing := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		existing[line] = struct{}{}
	}

	var out bytes.Buffer
	out.Write(content)

	for _, artifact := range artifacts {
		if _, ok := existing[artifact]; ok {
			continue
		}
		if out.Len() > 0 && out.Bytes()[out.Len()-1] != '\n' {
			out.WriteByte('\n')
		}
		out.WriteString(artifact)
		out.WriteByte('\n')
		existing[artifact] = struct{}{}
	}

	return os.WriteFile(path, out.Bytes(), 0o644)
}
