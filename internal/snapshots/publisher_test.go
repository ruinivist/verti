package snapshots

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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

func TestPublishSnapshotStoresWorktreeIdentityFieldsInMeta(t *testing.T) {
	scopeDir := t.TempDir()
	sha := "id123"

	publishedPath, err := PublishSnapshot(scopeDir, sha, nil, Meta{
		CommitSHA:               sha,
		WorktreeID:              "main",
		WorktreePathFingerprint: "fp-123",
	})
	if err != nil {
		t.Fatalf("PublishSnapshot() error = %v", err)
	}

	metaRaw, err := os.ReadFile(filepath.Join(publishedPath, "meta.json"))
	if err != nil {
		t.Fatalf("read meta.json: %v", err)
	}

	var metaDoc map[string]any
	if err := json.Unmarshal(metaRaw, &metaDoc); err != nil {
		t.Fatalf("unmarshal meta.json: %v", err)
	}

	if metaDoc["worktree_id"] != "main" {
		t.Fatalf("meta.json worktree_id = %v, want %q", metaDoc["worktree_id"], "main")
	}
	if metaDoc["worktree_path_fingerprint"] != "fp-123" {
		t.Fatalf("meta.json worktree_path_fingerprint = %v, want %q", metaDoc["worktree_path_fingerprint"], "fp-123")
	}
}

func TestPublishSnapshotConcurrentSameWorktreeAndSHAUsesSingleCanonicalPublishAndNarrowLock(t *testing.T) {
	scopeDir := t.TempDir()
	sha := "race-123"

	entriesA := []artifacts.ManifestEntry{
		{Path: "a.txt", Kind: artifacts.ArtifactKindFile, Status: artifacts.ArtifactStatusPresent},
	}
	entriesB := []artifacts.ManifestEntry{
		{Path: "b.txt", Kind: artifacts.ArtifactKindFile, Status: artifacts.ArtifactStatusPresent},
	}

	firstAtRenameHook := make(chan struct{})
	releaseFirst := make(chan struct{})
	var hookCalls atomic.Int32

	origHook := beforePublishRenameHook
	beforePublishRenameHook = func(_, _ string) error {
		if hookCalls.Add(1) == 1 {
			close(firstAtRenameHook)
			<-releaseFirst
		}
		return nil
	}
	t.Cleanup(func() { beforePublishRenameHook = origHook })

	errCh := make(chan error, 2)
	go func() {
		_, err := PublishSnapshot(scopeDir, sha, entriesA, Meta{CommitSHA: sha, WorktreeID: "main"})
		errCh <- err
	}()

	select {
	case <-firstAtRenameHook:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first publisher to reach pre-rename hook")
	}

	go func() {
		_, err := PublishSnapshot(scopeDir, sha, entriesB, Meta{CommitSHA: sha, WorktreeID: "main"})
		errCh <- err
	}()

	// The second publisher should be able to stage under .tmp before lock acquisition (narrow lock).
	tmpRoot := filepath.Join(scopeDir, "snapshots", ".tmp")
	deadline := time.Now().Add(2 * time.Second)
	sawMultipleTempDirs := false
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(tmpRoot)
		if err == nil {
			tmpCount := 0
			for _, e := range entries {
				if e.IsDir() {
					tmpCount++
				}
			}
			if tmpCount >= 2 {
				sawMultipleTempDirs = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sawMultipleTempDirs {
		t.Fatalf("expected concurrent temp staging before publish lock; did not observe >=2 temp dirs")
	}

	close(releaseFirst)

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent PublishSnapshot returned error: %v", err)
		}
	}

	publishedPath := filepath.Join(scopeDir, "snapshots", sha)
	if _, err := os.Stat(publishedPath); err != nil {
		t.Fatalf("expected canonical published snapshot at %q: %v", publishedPath, err)
	}

	tmpEntries, err := os.ReadDir(tmpRoot)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read .tmp dir: %v", err)
	}
	tmpDirCount := 0
	for _, e := range tmpEntries {
		if e.IsDir() {
			tmpDirCount++
		}
	}
	if tmpDirCount != 0 {
		t.Fatalf("expected temp dirs to be cleaned after concurrent publish, found %d", tmpDirCount)
	}
}
