package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"verti/internal/artifacts"
	"verti/internal/config"
	"verti/internal/git"
	"verti/internal/identity"
	"verti/internal/logging"
	"verti/internal/snapshots"
	"verti/internal/store"
)

var writeObjectFn = store.WriteObject

// RunSnapshot captures configured artifact state for current HEAD.
func RunSnapshot(workingDir string) error {
	return runSnapshot(workingDir, os.Stderr)
}

func runSnapshot(workingDir string, stderr io.Writer) error {
	repoRoot, err := git.RepoRoot(workingDir)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	commonGitDir, err := git.CommonGitDir(workingDir)
	if err != nil {
		return fmt.Errorf("resolve common git dir: %w", err)
	}
	headSHA, err := git.HeadSHA(workingDir)
	if err != nil {
		return fmt.Errorf("resolve HEAD sha: %w", err)
	}
	branch, err := git.CurrentBranch(workingDir)
	if err != nil {
		return fmt.Errorf("resolve current branch: %w", err)
	}

	cfgPath := filepath.Join(commonGitDir, "verti.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if !cfg.Enabled {
		return nil
	}
	if cfg.RepoID == "" {
		return fmt.Errorf("config %q missing repo_id; run `verti init`", cfgPath)
	}

	manifestEntries, err := artifacts.BuildManifestEntries(repoRoot, cfg.Artifacts)
	if err != nil {
		return fmt.Errorf("build artifact manifest entries: %w", err)
	}

	storeRoot, err := expandStoreRoot(cfg.StoreRoot)
	if err != nil {
		return err
	}

	writeManifestObjects(repoRoot, storeRoot, cfg.RepoID, cfg.MaxFileSizeMB, manifestEntries, stderr)

	worktreeID, err := identity.ResolveWorktreeIdentity(workingDir)
	if err != nil {
		return fmt.Errorf("resolve worktree identity: %w", err)
	}

	if err := cleanupExpiredQuarantineSessions(storeRoot, cfg.RepoID, nowUTC()); err != nil {
		logging.Warnf(stderr, "warning: unable to clean expired quarantine sessions: %v", err)
	}

	scopeDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", worktreeID.WorktreeID)
	meta := snapshots.Meta{
		CommitSHA:               headSHA,
		Branch:                  branch,
		WorktreeID:              worktreeID.WorktreeID,
		WorktreePathFingerprint: worktreeID.WorktreePathFingerprint,
	}
	if _, err := snapshots.PublishSnapshot(scopeDir, headSHA, manifestEntries, meta); err != nil {
		return fmt.Errorf("publish snapshot: %w", err)
	}
	if err := cleanupWorktreeQuarantineSessions(storeRoot, cfg.RepoID, worktreeID.WorktreeID); err != nil {
		logging.Warnf(stderr, "warning: unable to clean worktree quarantine sessions: %v", err)
	}

	return nil
}

func expandStoreRoot(storeRoot string) (string, error) {
	if strings.HasPrefix(storeRoot, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home for store_root %q: %w", storeRoot, err)
		}
		return filepath.Join(home, strings.TrimPrefix(storeRoot, "~"+string(filepath.Separator))), nil
	}
	if storeRoot == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home for store_root %q: %w", storeRoot, err)
		}
		return home, nil
	}
	if filepath.IsAbs(storeRoot) {
		return storeRoot, nil
	}
	abs, err := filepath.Abs(storeRoot)
	if err != nil {
		return "", fmt.Errorf("resolve absolute store_root %q: %w", storeRoot, err)
	}
	return abs, nil
}

func writeManifestObjects(repoRoot, storeRoot, repoID string, maxFileSizeMB int, entries []artifacts.ManifestEntry, stderr io.Writer) {
	maxFileBytes := int64(maxFileSizeMB) * 1024 * 1024
	objectsDir := filepath.Join(storeRoot, "repos", repoID, "objects")

	for i := range entries {
		entry := &entries[i]
		if entry.Kind != artifacts.ArtifactKindFile || entry.Status != artifacts.ArtifactStatusPresent {
			continue
		}

		if entry.Size > maxFileBytes {
			entry.Status = artifacts.ArtifactStatusSkipped
			entry.Hash = ""
			logging.Warnf(stderr, "warning: skipping artifact %q: size %d bytes exceeds max_file_size_mb=%d", entry.Path, entry.Size, maxFileSizeMB)
			continue
		}

		filePath := filepath.Join(repoRoot, filepath.FromSlash(entry.Path))
		data, err := os.ReadFile(filePath)
		if err != nil {
			logging.Warnf(stderr, "warning: unable to read artifact file %q for object store write: %v", entry.Path, err)
			continue
		}

		if _, err := writeObjectFn(objectsDir, data); err != nil {
			logging.Warnf(stderr, "warning: unable to write object for artifact %q: %v", entry.Path, err)
			continue
		}
	}
}
