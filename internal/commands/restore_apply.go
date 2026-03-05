package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"verti/internal/artifacts"
	"verti/internal/restoreplan"
)

func applyRestorePlan(ctx restoreApplyContext) error {
	entriesByPath := make(map[string]artifacts.ManifestEntry, len(ctx.Manifest))
	for _, entry := range ctx.Manifest {
		entriesByPath[entry.Path] = entry
	}

	objectsDir := filepath.Join(ctx.StoreRoot, "repos", ctx.RepoID, "objects")

	for _, op := range ctx.Plan {
		switch op.Type {
		case restoreplan.OpMkdir:
			if err := ensureDirectory(op.Path, op.AbsPath); err != nil {
				return err
			}
		case restoreplan.OpWriteFile:
			entry, ok := entriesByPath[op.Path]
			if !ok {
				return fmt.Errorf("missing manifest entry for restore file op %q", op.Path)
			}
			if err := writeRestoredFile(op.Path, op.AbsPath, entry, objectsDir); err != nil {
				return err
			}
		case restoreplan.OpWriteSymlink:
			entry, ok := entriesByPath[op.Path]
			if !ok {
				return fmt.Errorf("missing manifest entry for restore symlink op %q", op.Path)
			}
			if err := writeRestoredSymlink(op.Path, op.AbsPath, entry); err != nil {
				return err
			}
		case restoreplan.OpRemove:
			if err := removePath(op.Path, op.AbsPath); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported restore operation type %q", op.Type)
		}
	}

	return nil
}

func ensureDirectory(relPath, absPath string) error {
	info, err := os.Lstat(absPath)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		if err := removePath(relPath, absPath); err != nil {
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

func writeRestoredFile(relPath, absPath string, entry artifacts.ManifestEntry, objectsDir string) error {
	if entry.Kind != artifacts.ArtifactKindFile {
		return fmt.Errorf("manifest entry for %q is kind %q, expected file", relPath, entry.Kind)
	}
	if entry.Hash == "" {
		return fmt.Errorf("manifest entry for %q missing object hash", relPath)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %q: %w", relPath, err)
	}

	if _, err := os.Lstat(absPath); err == nil {
		if err := removePath(relPath, absPath); err != nil {
			return err
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

func writeRestoredSymlink(relPath, absPath string, entry artifacts.ManifestEntry) error {
	if entry.Kind != artifacts.ArtifactKindSymlink {
		return fmt.Errorf("manifest entry for %q is kind %q, expected symlink", relPath, entry.Kind)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory for symlink %q: %w", relPath, err)
	}

	if _, err := os.Lstat(absPath); err == nil {
		if err := removePath(relPath, absPath); err != nil {
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

func removePath(relPath, absPath string) error {
	if err := os.RemoveAll(absPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove path %q: %w", relPath, err)
	}
	return nil
}
