package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"verti/internal/config"
)

func TestLoadContextConfigOnlyPopulatesConfigAndGitCommonDir(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepo(t)

	ctx, err := LoadContext(repoDir, []ContextField{ContextFieldConfig})
	if err != nil {
		t.Fatalf("LoadContext(config) error = %v", err)
	}

	if ctx.Config == nil {
		t.Fatalf("expected config to be populated")
	}
	if ctx.Git == nil {
		t.Fatalf("expected git metadata to be populated")
	}
	if ctx.Git.CommonGitDir == "" {
		t.Fatalf("expected common git dir to be populated")
	}
	if ctx.Git.RepoRoot != "" {
		t.Fatalf("repo root should be empty when not requested; got %q", ctx.Git.RepoRoot)
	}
	if ctx.Store != nil || ctx.Worktree != nil || ctx.Paths != nil {
		t.Fatalf("unexpected derived context population for config-only request")
	}

	wantConfigPath := filepath.Join(repoDir, ".git", "verti.toml")
	if ctx.Config.Path != wantConfigPath {
		t.Fatalf("config path = %q, want %q", ctx.Config.Path, wantConfigPath)
	}
}

func TestLoadContextStorePathsPopulatesDerivedValues(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-context-paths",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	ctx, err := LoadContext(repoDir, []ContextField{ContextFieldStorePaths})
	if err != nil {
		t.Fatalf("LoadContext(store paths) error = %v", err)
	}

	if ctx.Config == nil || ctx.Store == nil || ctx.Worktree == nil || ctx.Paths == nil {
		t.Fatalf("expected config/store/worktree/paths to be populated")
	}

	wantRepoDir := filepath.Join(storeRoot, "repos", cfg.RepoID)
	if ctx.Paths.RepoDir != wantRepoDir {
		t.Fatalf("repo dir = %q, want %q", ctx.Paths.RepoDir, wantRepoDir)
	}

	wantScope := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main")
	if ctx.Paths.WorktreeScopeDir != wantScope {
		t.Fatalf("worktree scope dir = %q, want %q", ctx.Paths.WorktreeScopeDir, wantScope)
	}
	if ctx.Paths.SnapshotsDir != filepath.Join(wantScope, "snapshots") {
		t.Fatalf("snapshots dir mismatch: %q", ctx.Paths.SnapshotsDir)
	}
	if ctx.Paths.OrphansDir != filepath.Join(wantScope, "orphans") {
		t.Fatalf("orphans dir mismatch: %q", ctx.Paths.OrphansDir)
	}
	if ctx.Paths.ObjectsDir != filepath.Join(wantRepoDir, "objects") {
		t.Fatalf("objects dir mismatch: %q", ctx.Paths.ObjectsDir)
	}
}

func TestLoadContextStorePathsRequiresRepoIDWhenEnabled(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	_, err := LoadContext(repoDir, []ContextField{ContextFieldStorePaths})
	if err == nil {
		t.Fatalf("expected error for enabled config without repo_id")
	}
	if !strings.Contains(err.Error(), "config missing repo_id") {
		t.Fatalf("expected missing repo_id error, got %v", err)
	}
}

func TestLoadContextDisabledConfigSkipsStoreDerivedFields(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "",
		Enabled:       false,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	ctx, err := LoadContext(repoDir, []ContextField{ContextFieldStorePaths})
	if err != nil {
		t.Fatalf("LoadContext(disabled config) error = %v", err)
	}

	if ctx.Config == nil {
		t.Fatalf("expected config to be populated")
	}
	if ctx.Config.Value.Enabled {
		t.Fatalf("expected disabled config")
	}
	if ctx.Store != nil || ctx.Worktree != nil || ctx.Paths != nil {
		t.Fatalf("expected store/worktree/paths to be skipped for disabled config")
	}
}
