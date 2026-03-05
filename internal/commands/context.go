package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"verti/internal/config"
	"verti/internal/git"
	"verti/internal/identity"
)

// ContextField selects which context sections to populate.
type ContextField int

const (
	ContextFieldRepoRoot ContextField = iota
	ContextFieldConfig
	ContextFieldStoreRoot
	ContextFieldWorktreeIdentity
	ContextFieldStorePaths
)

// Context holds shared command bootstrap metadata.
type Context struct {
	WorkingDir string
	Git        *ContextGit
	Config     *ContextConfig
	Store      *ContextStore
	Worktree   *identity.WorktreeIdentity
	Paths      *ContextPaths
}

type ContextGit struct {
	RepoRoot     string
	CommonGitDir string
}

type ContextConfig struct {
	Path  string
	Value config.Config
}

type ContextStore struct {
	Root string
}

type ContextPaths struct {
	RepoDir          string
	WorktreeScopeDir string
	SnapshotsDir     string
	OrphansDir       string
	ObjectsDir       string
}

// LoadContext populates only requested fields and implied dependencies.
func LoadContext(workingDir string, fields []ContextField) (Context, error) {
	ctx := Context{WorkingDir: workingDir}
	req := make(map[ContextField]struct{}, len(fields))
	for _, field := range fields {
		req[field] = struct{}{}
	}

	needRepoRoot := hasContextField(req, ContextFieldRepoRoot)
	needConfig := hasContextField(req, ContextFieldConfig) ||
		hasContextField(req, ContextFieldStoreRoot) ||
		hasContextField(req, ContextFieldStorePaths)
	needStoreRoot := hasContextField(req, ContextFieldStoreRoot) ||
		hasContextField(req, ContextFieldStorePaths)
	needWorktree := hasContextField(req, ContextFieldWorktreeIdentity) ||
		hasContextField(req, ContextFieldStorePaths)
	needStorePaths := hasContextField(req, ContextFieldStorePaths)

	if needRepoRoot {
		repoRoot, err := git.RepoRoot(workingDir)
		if err != nil {
			return Context{}, fmt.Errorf("resolve repo root: %w", err)
		}
		ctx.Git = &ContextGit{RepoRoot: repoRoot}
	}

	if needConfig {
		commonGitDir, err := git.CommonGitDir(workingDir)
		if err != nil {
			return Context{}, fmt.Errorf("resolve common git dir: %w", err)
		}

		if ctx.Git == nil {
			ctx.Git = &ContextGit{}
		}
		ctx.Git.CommonGitDir = commonGitDir

		cfgPath := filepath.Join(commonGitDir, "verti.toml")
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return Context{}, fmt.Errorf("load config: %w", err)
		}

		ctx.Config = &ContextConfig{
			Path:  cfgPath,
			Value: cfg,
		}

		// Commands using config-disabled mode should be able to no-op without
		// requiring store/worktree/repo_id resolution.
		if !cfg.Enabled {
			return ctx, nil
		}
	}

	if needStoreRoot {
		storeRoot, err := expandStoreRoot(ctx.Config.Value.StoreRoot)
		if err != nil {
			return Context{}, err
		}
		ctx.Store = &ContextStore{Root: storeRoot}
	}

	if needWorktree {
		worktreeID, err := identity.ResolveWorktreeIdentity(workingDir)
		if err != nil {
			return Context{}, fmt.Errorf("resolve worktree identity: %w", err)
		}
		ctx.Worktree = &worktreeID
	}

	if needStorePaths {
		repoID := ctx.Config.Value.RepoID
		if repoID == "" {
			return Context{}, fmt.Errorf("config missing repo_id; run `verti init`")
		}

		repoDir := filepath.Join(ctx.Store.Root, "repos", repoID)
		worktreeScopeDir := filepath.Join(repoDir, "worktrees", ctx.Worktree.WorktreeID)
		ctx.Paths = &ContextPaths{
			RepoDir:          repoDir,
			WorktreeScopeDir: worktreeScopeDir,
			SnapshotsDir:     filepath.Join(worktreeScopeDir, "snapshots"),
			OrphansDir:       filepath.Join(worktreeScopeDir, "orphans"),
			ObjectsDir:       filepath.Join(repoDir, "objects"),
		}
	}

	return ctx, nil
}

func hasContextField(req map[ContextField]struct{}, field ContextField) bool {
	_, ok := req[field]
	return ok
}

// resolves to abolute path of store from config, handling is basically for
// relative paths beginning with ~
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
