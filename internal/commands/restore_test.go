package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"verti/internal/cli"
	"verti/internal/config"
)

func TestRunRestoreMissingSnapshotNoOpLeavesFilesUnchanged(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-noop",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	before, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read pre-restore progress.md: %v", err)
	}

	var stderr bytes.Buffer
	if err := runRestore(repoDir, []string{"deadbeef"}, &stderr); err != nil {
		t.Fatalf("runRestore() error = %v", err)
	}

	after, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read post-restore progress.md: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("restore no-op changed artifact contents")
	}

	if _, err := os.Stat(storeRoot); !os.IsNotExist(err) {
		t.Fatalf("restore no-op should not create store filesystem changes, stat err=%v", err)
	}
}

func TestRunRestoreArgumentContract(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-args",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	err := runRestore(repoDir, []string{}, &stderr)
	var usageErr *cli.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected usage error for missing target, got %v", err)
	}

	err = runRestore(repoDir, []string{"--orphan"}, &stderr)
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected usage error for --orphan without id, got %v", err)
	}

	if err := runRestore(repoDir, []string{"--orphan", "orphan-1"}, &stderr); err != nil {
		t.Fatalf("expected --orphan mode to satisfy argument contract, got %v", err)
	}

	if err := runRestore(repoDir, []string{"cafebabe"}, &stderr); err != nil {
		t.Fatalf("expected sha target to satisfy argument contract, got %v", err)
	}

	if strings.Contains(stderr.String(), "panic") {
		t.Fatalf("unexpected stderr output: %q", stderr.String())
	}
}

func TestRunRestoreCreatesOrphanBeforeDecisionPathExecutes(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-orphan-order",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var snapshotStderr bytes.Buffer
	if err := runSnapshot(repoDir, &snapshotStderr); err != nil {
		t.Fatalf("runSnapshot() error = %v", err)
	}
	targetSHA := runGit(t, repoDir, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("local divergence\n"), 0o644); err != nil {
		t.Fatalf("write progress.md divergence: %v", err)
	}

	var hookCalled bool
	origHook := beforeRestoreDecisionHook
	beforeRestoreDecisionHook = func(ctx restoreDecisionContext) error {
		hookCalled = true

		if strings.TrimSpace(ctx.OrphanID) == "" {
			t.Fatalf("restore decision context missing orphan id")
		}

		orphanPath := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "orphans", ctx.OrphanID)
		if _, err := os.Stat(filepath.Join(orphanPath, "meta.json")); err != nil {
			t.Fatalf("expected orphan snapshot to exist before decision hook: %v", err)
		}
		return nil
	}
	t.Cleanup(func() { beforeRestoreDecisionHook = origHook })

	var restoreStderr bytes.Buffer
	if err := runRestore(repoDir, []string{targetSHA}, &restoreStderr); err != nil {
		t.Fatalf("runRestore() error = %v", err)
	}
	if !hookCalled {
		t.Fatalf("expected restore decision hook to be called")
	}
}

func TestRunRestoreOrphanMetaIncludesTimestampAndTriggeringCheckoutSHA(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-orphan-meta",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var snapshotStderr bytes.Buffer
	if err := runSnapshot(repoDir, &snapshotStderr); err != nil {
		t.Fatalf("runSnapshot() error = %v", err)
	}
	targetSHA := runGit(t, repoDir, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("local divergence\n"), 0o644); err != nil {
		t.Fatalf("write progress.md divergence: %v", err)
	}

	origHook := beforeRestoreDecisionHook
	beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }
	t.Cleanup(func() { beforeRestoreDecisionHook = origHook })

	var restoreStderr bytes.Buffer
	if err := runRestore(repoDir, []string{targetSHA}, &restoreStderr); err != nil {
		t.Fatalf("runRestore() error = %v", err)
	}

	orphansRoot := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "orphans")
	entries, err := os.ReadDir(orphansRoot)
	if err != nil {
		t.Fatalf("read orphans root: %v", err)
	}

	var orphanIDs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		orphanIDs = append(orphanIDs, entry.Name())
	}
	if len(orphanIDs) != 1 {
		t.Fatalf("expected exactly one orphan snapshot, found %d", len(orphanIDs))
	}

	metaRaw, err := os.ReadFile(filepath.Join(orphansRoot, orphanIDs[0], "meta.json"))
	if err != nil {
		t.Fatalf("read orphan meta.json: %v", err)
	}

	var metaDoc map[string]any
	if err := json.Unmarshal(metaRaw, &metaDoc); err != nil {
		t.Fatalf("unmarshal orphan meta.json: %v", err)
	}

	createdAt, _ := metaDoc["created_at"].(string)
	if createdAt == "" {
		t.Fatalf("orphan meta missing created_at")
	}
	if _, err := time.Parse(time.RFC3339, createdAt); err != nil {
		t.Fatalf("orphan created_at is not RFC3339: %q (%v)", createdAt, err)
	}

	if metaDoc["triggering_checkout_sha"] != targetSHA {
		t.Fatalf("orphan triggering_checkout_sha = %v, want %q", metaDoc["triggering_checkout_sha"], targetSHA)
	}
}
