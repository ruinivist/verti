package snapshots

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"verti/internal/artifacts"
)

func TestPublishSnapshotCreatesSnapshotsPathForSHA(t *testing.T) {
	scopeDir := t.TempDir()
	sha := "abc123"

	entries := []artifacts.ManifestEntry{
		{Path: "progress.md", Kind: artifacts.ArtifactKindFile, Status: artifacts.ArtifactStatusPresent},
	}
	meta := Meta{
		CommitSHA: sha,
		Branch:    "main",
	}

	publishedPath, err := PublishSnapshot(scopeDir, sha, entries, meta)
	if err != nil {
		t.Fatalf("PublishSnapshot() error = %v", err)
	}

	wantPath := filepath.Join(scopeDir, "snapshots", sha)
	if publishedPath != wantPath {
		t.Fatalf("PublishSnapshot() path = %q, want %q", publishedPath, wantPath)
	}

	if _, err := os.Stat(filepath.Join(wantPath, "manifest.json")); err != nil {
		t.Fatalf("manifest.json missing at published path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantPath, "meta.json")); err != nil {
		t.Fatalf("meta.json missing at published path: %v", err)
	}
}

func TestPublishSnapshotFailureBeforeRenameLeavesNoVisiblePartialSnapshot(t *testing.T) {
	scopeDir := t.TempDir()
	sha := "deadbeef"

	origHook := beforePublishRenameHook
	beforePublishRenameHook = func(_, _ string) error {
		return errors.New("injected pre-rename failure")
	}
	t.Cleanup(func() { beforePublishRenameHook = origHook })

	_, err := PublishSnapshot(scopeDir, sha, nil, Meta{CommitSHA: sha})
	if err == nil {
		t.Fatalf("PublishSnapshot() expected failure from pre-rename hook")
	}

	publishedPath := filepath.Join(scopeDir, "snapshots", sha)
	if _, statErr := os.Stat(publishedPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no published snapshot dir, stat err=%v", statErr)
	}

	tmpRoot := filepath.Join(scopeDir, "snapshots", ".tmp")
	entries, readErr := os.ReadDir(tmpRoot)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read .tmp dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no leftover temp snapshot dirs, found %d", len(entries))
	}
}

func TestPublishSnapshotWritesSchemaVersionInMetaAndManifest(t *testing.T) {
	scopeDir := t.TempDir()
	sha := "ff00"

	publishedPath, err := PublishSnapshot(scopeDir, sha, nil, Meta{CommitSHA: sha})
	if err != nil {
		t.Fatalf("PublishSnapshot() error = %v", err)
	}

	manifestRaw, err := os.ReadFile(filepath.Join(publishedPath, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest.json: %v", err)
	}
	metaRaw, err := os.ReadFile(filepath.Join(publishedPath, "meta.json"))
	if err != nil {
		t.Fatalf("read meta.json: %v", err)
	}

	var manifestDoc map[string]any
	if err := json.Unmarshal(manifestRaw, &manifestDoc); err != nil {
		t.Fatalf("unmarshal manifest.json: %v", err)
	}
	var metaDoc map[string]any
	if err := json.Unmarshal(metaRaw, &metaDoc); err != nil {
		t.Fatalf("unmarshal meta.json: %v", err)
	}

	if _, ok := manifestDoc["schema_version"]; !ok {
		t.Fatalf("manifest.json missing schema_version field")
	}
	if _, ok := metaDoc["schema_version"]; !ok {
		t.Fatalf("meta.json missing schema_version field")
	}
}
