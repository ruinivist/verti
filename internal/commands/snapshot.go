package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"verti/internal/artifacts"
	"verti/internal/git"
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
	ctx, err := LoadContext(workingDir, []ContextField{
		ContextFieldRepoRoot,
		ContextFieldConfig,
		ContextFieldStoreRoot,
		ContextFieldWorktreeIdentity,
		ContextFieldStorePaths,
	})
	if err != nil {
		return err
	}
	cfg := ctx.Config.Value
	if !cfg.Enabled {
		return nil
	}

	headSHA, err := git.HeadSHA(workingDir)
	if err != nil {
		return fmt.Errorf("resolve HEAD sha: %w", err)
	}
	branch, err := git.CurrentBranch(workingDir)
	if err != nil {
		return fmt.Errorf("resolve current branch: %w", err)
	}

	repoRoot := ctx.Git.RepoRoot
	manifestEntries, err := artifacts.BuildManifestEntries(repoRoot, cfg.Artifacts)
	if err != nil {
		return fmt.Errorf("build artifact manifest entries: %w", err)
	}

	storeRoot := ctx.Store.Root

	writeManifestObjects(repoRoot, storeRoot, cfg.RepoID, cfg.MaxFileSizeMB, manifestEntries, stderr)

	worktreeID := *ctx.Worktree
	scopeDir := ctx.Paths.WorktreeScopeDir
	meta := snapshots.Meta{
		CommitSHA:               headSHA,
		Branch:                  branch,
		WorktreeID:              worktreeID.WorktreeID,
		WorktreePathFingerprint: worktreeID.WorktreePathFingerprint,
	}
	if _, err := snapshots.PublishSnapshot(scopeDir, headSHA, manifestEntries, meta); err != nil {
		return fmt.Errorf("publish snapshot: %w", err)
	}

	return nil
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
