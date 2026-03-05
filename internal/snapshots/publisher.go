package snapshots

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"verti/internal/artifacts"
)

const (
	manifestFilename = "manifest.json"
	metaFilename     = "meta.json"

	SchemaVersion = 1

	SnapshotKindNormal = "normal"
	SnapshotKindOrphan = "orphan"
)

var beforePublishRenameHook = func(_, _ string) error { return nil }

type Manifest struct {
	SchemaVersion int                       `json:"schema_version"`
	Entries       []artifacts.ManifestEntry `json:"entries"`
}

type Meta struct {
	SchemaVersion           int    `json:"schema_version"`
	CommitSHA               string `json:"commit_sha,omitempty"`
	Branch                  string `json:"branch,omitempty"`
	WorktreeID              string `json:"worktree_id,omitempty"`
	WorktreePathFingerprint string `json:"worktree_path_fingerprint,omitempty"`
	CreatedAt               string `json:"created_at,omitempty"`
	SnapshotKind            string `json:"snapshot_kind,omitempty"`
	OrphanID                string `json:"orphan_id,omitempty"`
	TriggeringCheckoutSHA   string `json:"triggering_checkout_sha,omitempty"`
}

// PublishSnapshot writes manifest/meta in a temporary snapshot dir and atomically
// publishes it to <scopeDir>/snapshots/<sha>.
func PublishSnapshot(scopeDir, sha string, entries []artifacts.ManifestEntry, meta Meta) (string, error) {
	return publish(scopeDir, "snapshots", sha, entries, meta)
}

// PublishOrphanSnapshot writes manifest/meta in a temporary orphan dir and
// atomically publishes it to <scopeDir>/orphans/<orphanID>.
func PublishOrphanSnapshot(scopeDir, orphanID string, entries []artifacts.ManifestEntry, meta Meta) (string, error) {
	orphanID = strings.TrimSpace(orphanID)
	if orphanID == "" {
		return "", fmt.Errorf("orphan id cannot be empty")
	}
	meta.SnapshotKind = SnapshotKindOrphan
	meta.OrphanID = orphanID
	return publish(scopeDir, "orphans", orphanID, entries, meta)
}

func publish(scopeDir, collectionDir, id string, entries []artifacts.ManifestEntry, meta Meta) (string, error) {
	publishRoot := filepath.Join(scopeDir, collectionDir)
	tmpRoot := filepath.Join(publishRoot, internalCollectionTmpDir)
	targetDir := filepath.Join(publishRoot, id)

	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return "", fmt.Errorf("create snapshot temp root %q: %w", tmpRoot, err)
	}

	tmpDir, err := os.MkdirTemp(tmpRoot, id+".")
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
	if meta.SnapshotKind == "" {
		meta.SnapshotKind = SnapshotKindNormal
	}
	if meta.CreatedAt == "" {
		meta.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	if err := writeJSONFile(filepath.Join(tmpDir, manifestFilename), manifest); err != nil {
		return "", err
	}
	if err := writeJSONFile(filepath.Join(tmpDir, metaFilename), meta); err != nil {
		return "", err
	}

	unlock, err := acquirePublishLock(publishRoot)
	if err != nil {
		return "", err
	}
	defer unlock()

	if _, err := os.Stat(targetDir); err == nil {
		// Another writer already published this snapshot for the same worktree/SHA.
		return targetDir, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat publish target %q: %w", targetDir, err)
	}

	if err := beforePublishRenameHook(tmpDir, targetDir); err != nil {
		return "", fmt.Errorf("pre-rename publish hook failed: %w", err)
	}

	if err := os.Rename(tmpDir, targetDir); err != nil {
		if _, statErr := os.Stat(targetDir); statErr == nil {
			// Another writer won the publish race while we were waiting for the lock.
			return targetDir, nil
		}
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

func acquirePublishLock(snapshotsDir string) (func(), error) {
	lockPath := filepath.Join(snapshotsDir, ".publish.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open publish lock %q: %w", lockPath, err)
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("acquire publish lock %q: %w", lockPath, err)
	}

	return func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}, nil
}
