package snapshots

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"verti/internal/artifacts"
)

const (
	manifestFilename = "manifest.json"
	metaFilename     = "meta.json"

	SchemaVersion = 1
)

var beforePublishRenameHook = func(_, _ string) error { return nil }

type Manifest struct {
	SchemaVersion int                       `json:"schema_version"`
	Entries       []artifacts.ManifestEntry `json:"entries"`
}

type Meta struct {
	SchemaVersion int    `json:"schema_version"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	Branch        string `json:"branch,omitempty"`
}

// PublishSnapshot writes manifest/meta in a temporary snapshot dir and atomically
// publishes it to <scopeDir>/snapshots/<sha>.
func PublishSnapshot(scopeDir, sha string, entries []artifacts.ManifestEntry, meta Meta) (string, error) {
	snapshotsDir := filepath.Join(scopeDir, "snapshots")
	tmpRoot := filepath.Join(snapshotsDir, ".tmp")
	targetDir := filepath.Join(snapshotsDir, sha)

	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return "", fmt.Errorf("create snapshot temp root %q: %w", tmpRoot, err)
	}

	tmpDir, err := os.MkdirTemp(tmpRoot, sha+".")
	if err != nil {
		return "", fmt.Errorf("create temp snapshot dir under %q: %w", tmpRoot, err)
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Entries:       entries,
	}
	if meta.SchemaVersion == 0 {
		meta.SchemaVersion = SchemaVersion
	}

	if err := writeJSONFile(filepath.Join(tmpDir, manifestFilename), manifest); err != nil {
		return "", err
	}
	if err := writeJSONFile(filepath.Join(tmpDir, metaFilename), meta); err != nil {
		return "", err
	}

	if err := beforePublishRenameHook(tmpDir, targetDir); err != nil {
		return "", fmt.Errorf("pre-rename publish hook failed: %w", err)
	}

	if err := os.Rename(tmpDir, targetDir); err != nil {
		return "", fmt.Errorf("publish snapshot dir %q -> %q: %w", tmpDir, targetDir, err)
	}

	cleanup = false
	return targetDir, nil
}

func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON for %q: %w", path, err)
	}
	raw = append(raw, '\n')

	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write JSON file %q: %w", path, err)
	}
	return nil
}
