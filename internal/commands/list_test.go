package commands

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"verti/internal/cli"
	"verti/internal/config"
	"verti/internal/snapshots"
)

func TestRunListOutputsTableForCurrentWorktreeOnly(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-list-current-worktree",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	mainScope := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main")
	_, err := snapshots.PublishSnapshot(mainScope, "aaa111", nil, snapshots.Meta{
		CommitSHA:    "aaa111",
		Branch:       "main",
		CreatedAt:    "2026-03-02T08:00:00Z",
		SnapshotKind: snapshots.SnapshotKindNormal,
	})
	if err != nil {
		t.Fatalf("publish main snapshot aaa111: %v", err)
	}
	_, err = snapshots.PublishSnapshot(mainScope, "bbb222", nil, snapshots.Meta{
		CommitSHA:    "bbb222",
		Branch:       "main",
		CreatedAt:    "2026-03-02T09:00:00Z",
		SnapshotKind: snapshots.SnapshotKindNormal,
	})
	if err != nil {
		t.Fatalf("publish main snapshot bbb222: %v", err)
	}

	otherScope := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "other")
	_, err = snapshots.PublishSnapshot(otherScope, "ccc333", nil, snapshots.Meta{
		CommitSHA:    "ccc333",
		Branch:       "feature",
		CreatedAt:    "2026-03-02T10:00:00Z",
		SnapshotKind: snapshots.SnapshotKindNormal,
	})
	if err != nil {
		t.Fatalf("publish other-worktree snapshot ccc333: %v", err)
	}

	var stdout bytes.Buffer
	if err := runList(repoDir, nil, &stdout); err != nil {
		t.Fatalf("runList() error = %v", err)
	}

	out := stdout.String()
	for _, column := range []string{"COMMIT", "BRANCH", "CREATED_AT", "KIND"} {
		if !strings.Contains(out, column) {
			t.Fatalf("expected %q column in list output, got %q", column, out)
		}
	}

	if !strings.Contains(out, "aaa111") || !strings.Contains(out, "bbb222") {
		t.Fatalf("expected main worktree snapshots in output, got %q", out)
	}
	if strings.Contains(out, "ccc333") {
		t.Fatalf("expected other-worktree snapshot to be filtered out, got %q", out)
	}
	if !strings.Contains(out, "2026-03-02T09:00:00Z") {
		t.Fatalf("expected created_at value in output, got %q", out)
	}
	if !strings.Contains(out, snapshots.SnapshotKindNormal) {
		t.Fatalf("expected snapshot kind in output, got %q", out)
	}
}

func TestRunListRejectsUnexpectedArgs(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-list-args",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stdout bytes.Buffer
	err := runList(repoDir, []string{"unexpected"}, &stdout)
	var usageErr *cli.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected usage error for unexpected list args, got %v", err)
	}
}

func TestRunListWithOrphansIncludesOrphanIDAndCheckoutSHA(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-list-orphans",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	mainScope := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main")
	_, err := snapshots.PublishSnapshot(mainScope, "abc123", nil, snapshots.Meta{
		CommitSHA:    "abc123",
		Branch:       "main",
		CreatedAt:    "2026-03-02T08:00:00Z",
		SnapshotKind: snapshots.SnapshotKindNormal,
	})
	if err != nil {
		t.Fatalf("publish main snapshot: %v", err)
	}

	_, err = snapshots.PublishOrphanSnapshot(mainScope, "orphan-xyz", nil, snapshots.Meta{
		CreatedAt:             "2026-03-02T09:00:00Z",
		TriggeringCheckoutSHA: "checkout-999",
		WorktreeID:            "main",
	})
	if err != nil {
		t.Fatalf("publish orphan snapshot: %v", err)
	}

	var stdout bytes.Buffer
	if err := runList(repoDir, []string{"--orphans"}, &stdout); err != nil {
		t.Fatalf("runList(--orphans) error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "ORPHAN_ID") || !strings.Contains(out, "TRIGGERING_CHECKOUT_SHA") {
		t.Fatalf("expected orphan columns in list --orphans output, got %q", out)
	}
	if !strings.Contains(out, "orphan-xyz") {
		t.Fatalf("expected orphan id in list output, got %q", out)
	}
	if !strings.Contains(out, "checkout-999") {
		t.Fatalf("expected triggering checkout sha in list output, got %q", out)
	}
}
