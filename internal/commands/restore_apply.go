package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"verti/internal/artifacts"
	"verti/internal/restoreplan"
)

var nowUTC = func() time.Time { return time.Now().UTC() }

func applyRestorePlan(ctx restoreApplyContext) error {
	entriesByPath := make(map[string]artifacts.ManifestEntry, len(ctx.Manifest))
	for _, entry := range ctx.Manifest {
		entriesByPath[entry.Path] = entry
	}

	objectsDir := filepath.Join(ctx.StoreRoot, "repos", ctx.RepoID, "objects")
	quarantine := newQuarantineSession(ctx.StoreRoot, ctx.RepoID, ctx.WorktreeID, ctx.TargetSHA, ctx.OrphanID)

	for _, op := range ctx.Plan {
		switch op.Type {
		case restoreplan.OpMkdir:
			if err := ensureDirectory(op.Path, op.AbsPath, quarantine); err != nil {
				return err
			}
		case restoreplan.OpWriteFile:
			entry, ok := entriesByPath[op.Path]
			if !ok {
				return fmt.Errorf("missing manifest entry for restore file op %q", op.Path)
			}
			if err := writeRestoredFile(op.Path, op.AbsPath, entry, objectsDir, quarantine); err != nil {
				return err
			}
		case restoreplan.OpWriteSymlink:
			entry, ok := entriesByPath[op.Path]
			if !ok {
				return fmt.Errorf("missing manifest entry for restore symlink op %q", op.Path)
			}
			if err := writeRestoredSymlink(op.Path, op.AbsPath, entry, quarantine); err != nil {
				return err
			}
		case restoreplan.OpRemove:
			if err := quarantine.Move(op.Path, op.AbsPath); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported restore operation type %q", op.Type)
		}
	}

	if err := quarantine.Finalize(); err != nil {
		return err
	}
	return nil
}

func ensureDirectory(relPath, absPath string, quarantine *quarantineSession) error {
	info, err := os.Lstat(absPath)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		if err := quarantine.Move(relPath, absPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat restore mkdir target %q: %w", relPath, err)
	}

	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return fmt.Errorf("create restore directory %q: %w", relPath, err)
	}
	return nil
}

func writeRestoredFile(relPath, absPath string, entry artifacts.ManifestEntry, objectsDir string, quarantine *quarantineSession) error {
	if entry.Kind != artifacts.ArtifactKindFile {
		return fmt.Errorf("manifest entry for %q is kind %q, expected file", relPath, entry.Kind)
	}
	if entry.Hash == "" {
		return fmt.Errorf("manifest entry for %q missing object hash", relPath)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %q: %w", relPath, err)
	}

	info, err := os.Lstat(absPath)
	if err == nil {
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			if err := quarantine.Move(relPath, absPath); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat restore file target %q: %w", relPath, err)
	}

	objectPath := filepath.Join(objectsDir, entry.Hash)
	data, err := os.ReadFile(objectPath)
	if err != nil {
		return fmt.Errorf("read object %q for restore path %q: %w", entry.Hash, relPath, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(absPath), ".verti-restore-file-*")
	if err != nil {
		return fmt.Errorf("create temp file for restore path %q: %w", relPath, err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp restore file for %q: %w", relPath, err)
	}

	perm := os.FileMode(entry.Mode & 0o777)
	if perm == 0 {
		perm = 0o644
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp restore file for %q: %w", relPath, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp restore file for %q: %w", relPath, err)
	}

	if err := os.Rename(tmpPath, absPath); err != nil {
		return fmt.Errorf("publish restored file %q: %w", relPath, err)
	}

	cleanup = false
	return nil
}

func writeRestoredSymlink(relPath, absPath string, entry artifacts.ManifestEntry, quarantine *quarantineSession) error {
	if entry.Kind != artifacts.ArtifactKindSymlink {
		return fmt.Errorf("manifest entry for %q is kind %q, expected symlink", relPath, entry.Kind)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory for symlink %q: %w", relPath, err)
	}

	if _, err := os.Lstat(absPath); err == nil {
		if err := quarantine.Move(relPath, absPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat restore symlink target %q: %w", relPath, err)
	}

	if err := os.Symlink(entry.LinkTarget, absPath); err != nil {
		return fmt.Errorf("create restored symlink %q -> %q: %w", relPath, entry.LinkTarget, err)
	}
	return nil
}

type quarantineSession struct {
	storeRoot  string
	repoID     string
	worktreeID string
	targetSHA  string
	orphanID   string

	started   bool
	sessionID string
	root      string
	pathsRoot string
	moved     []string
}

func newQuarantineSession(storeRoot, repoID, worktreeID, targetSHA, orphanID string) *quarantineSession {
	return &quarantineSession{
		storeRoot:  storeRoot,
		repoID:     repoID,
		worktreeID: worktreeID,
		targetSHA:  targetSHA,
		orphanID:   orphanID,
	}
}

func (q *quarantineSession) Move(relPath, absPath string) error {
	info, err := os.Lstat(absPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat path for quarantine %q: %w", relPath, err)
	}

	if err := q.ensureStarted(); err != nil {
		return err
	}

	dest := filepath.Join(q.pathsRoot, filepath.FromSlash(relPath))
	if info.IsDir() {
		if _, err := os.Stat(dest); err == nil {
			// Descendants were already moved under this destination.
			if err := os.RemoveAll(absPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove already-quarantined source dir %q: %w", relPath, err)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat quarantine destination %q: %w", relPath, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create quarantine parent dir for %q: %w", relPath, err)
	}

	if _, err := os.Lstat(dest); err == nil {
		// Keep source data by creating a unique fallback destination.
		dest = q.uniquePath(dest)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat quarantine destination %q: %w", relPath, err)
	}

	if err := os.Rename(absPath, dest); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("move %q into quarantine: %w", relPath, err)
	}

	q.moved = append(q.moved, relPath)
	return nil
}

func (q *quarantineSession) Finalize() error {
	if !q.started {
		return nil
	}

	sort.Strings(q.moved)
	meta := struct {
		SchemaVersion     int      `json:"schema_version"`
		CreatedAt         string   `json:"created_at"`
		RepoID            string   `json:"repo_id"`
		WorktreeID        string   `json:"worktree_id"`
		TargetSnapshotSHA string   `json:"target_snapshot_sha"`
		OrphanID          string   `json:"orphan_id,omitempty"`
		MovedPaths        []string `json:"moved_paths"`
	}{
		SchemaVersion:     1,
		CreatedAt:         nowUTC().Format(time.RFC3339),
		RepoID:            q.repoID,
		WorktreeID:        q.worktreeID,
		TargetSnapshotSHA: q.targetSHA,
		OrphanID:          q.orphanID,
		MovedPaths:        q.moved,
	}

	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal quarantine metadata: %w", err)
	}
	raw = append(raw, '\n')

	metaPath := filepath.Join(q.root, "meta.json")
	if err := os.WriteFile(metaPath, raw, 0o644); err != nil {
		return fmt.Errorf("write quarantine metadata %q: %w", metaPath, err)
	}
	return nil
}

func (q *quarantineSession) ensureStarted() error {
	if q.started {
		return nil
	}

	q.sessionID = fmt.Sprintf("%d", nowUTC().UnixNano())
	q.root = filepath.Join(q.storeRoot, "repos", q.repoID, "quarantine", q.sessionID)
	q.pathsRoot = filepath.Join(q.root, "paths")

	if err := os.MkdirAll(q.pathsRoot, 0o755); err != nil {
		return fmt.Errorf("create quarantine session %q: %w", q.root, err)
	}

	q.started = true
	return nil
}

func (q *quarantineSession) uniquePath(base string) string {
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.dup%d", base, i)
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
