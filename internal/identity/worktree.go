package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"verti/internal/git"
)

type WorktreeIdentity struct {
	WorktreeID              string
	WorktreePathFingerprint string
}

// ResolveWorktreeIdentity derives worktree identity from git dir structure and path fingerprint.
func ResolveWorktreeIdentity(workingDir string) (WorktreeIdentity, error) {
	gitDir, err := git.GitDir(workingDir)
	if err != nil {
		return WorktreeIdentity{}, fmt.Errorf("resolve git dir: %w", err)
	}
	commonGitDir, err := git.CommonGitDir(workingDir)
	if err != nil {
		return WorktreeIdentity{}, fmt.Errorf("resolve common git dir: %w", err)
	}

	worktreeID := filepath.Base(gitDir)

	// TODO: this is a bug as I cannot have a worktree called as "main" as that would
	// collide with base repo
	if filepath.Clean(gitDir) == filepath.Clean(commonGitDir) {
		worktreeID = "main"
	}
	if worktreeID == "" || worktreeID == "." || worktreeID == string(filepath.Separator) {
		return WorktreeIdentity{}, fmt.Errorf("failed to derive worktree id from git dir %q", gitDir)
	}

	repoRoot, err := git.RepoRoot(workingDir)
	if err != nil {
		return WorktreeIdentity{}, fmt.Errorf("resolve repo root: %w", err)
	}
	fingerprint, err := PathFingerprint(repoRoot)
	if err != nil {
		return WorktreeIdentity{}, err
	}

	return WorktreeIdentity{
		WorktreeID:              worktreeID,
		WorktreePathFingerprint: fingerprint,
	}, nil
}

// PathFingerprint returns a stable fingerprint for the canonical absolute path.
func PathFingerprint(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path for %q: %w", path, err)
	}

	// if it's a symlink, resolve
	canonicalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// Fall back to absolute path if symlink eval is unavailable; this still keeps deterministic behavior.
		canonicalPath = absPath
	}

	sum := sha256.Sum256([]byte(filepath.Clean(canonicalPath)))
	return hex.EncodeToString(sum[:]), nil
}
