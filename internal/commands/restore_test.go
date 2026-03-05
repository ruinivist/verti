package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"verti/internal/artifacts"
	"verti/internal/cli"
	"verti/internal/config"
	"verti/internal/snapshots"
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
		if entry.Name() == ".tmp" {
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

func TestRunRestorePromptShownOnlyInInteractiveContext(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-prompt-visibility",
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

	origDecisionHook := beforeRestoreDecisionHook
	beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }
	t.Cleanup(func() { beforeRestoreDecisionHook = origDecisionHook })

	t.Run("interactive", func(t *testing.T) {
		tty := newFakeTTY("n\n")

		origOpenTTY := openPromptTTY
		openPromptTTY = func() (io.ReadWriteCloser, error) { return tty, nil }
		t.Cleanup(func() { openPromptTTY = origOpenTTY })

		var stderr bytes.Buffer
		if err := runRestore(repoDir, []string{targetSHA}, &stderr); err != nil {
			t.Fatalf("runRestore() error = %v", err)
		}
		if !strings.Contains(tty.Output(), "Restore artifacts? [y/N]") {
			t.Fatalf("expected prompt output on interactive tty, got %q", tty.Output())
		}
	})

	t.Run("non_interactive", func(t *testing.T) {
		promptCalled := false

		origOpenTTY := openPromptTTY
		openPromptTTY = func() (io.ReadWriteCloser, error) { return nil, fmt.Errorf("no tty") }
		t.Cleanup(func() { openPromptTTY = origOpenTTY })

		origPromptFn := promptRestoreConfirmationFn
		promptRestoreConfirmationFn = func(_ io.ReadWriter, _, _ string) (bool, error) {
			promptCalled = true
			return true, nil
		}
		t.Cleanup(func() { promptRestoreConfirmationFn = origPromptFn })

		var stderr bytes.Buffer
		if err := runRestore(repoDir, []string{targetSHA}, &stderr); err != nil {
			t.Fatalf("runRestore() error = %v", err)
		}
		if promptCalled {
			t.Fatalf("prompt should not be called when tty is unavailable")
		}
	})
}

func TestRunRestoreDeclineExitsCleanlyWarnsAndDoesNotApply(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-decline",
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

	beforeContent, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read pre-restore progress.md: %v", err)
	}

	origDecisionHook := beforeRestoreDecisionHook
	beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }
	t.Cleanup(func() { beforeRestoreDecisionHook = origDecisionHook })

	tty := newFakeTTY("N\n")
	origOpenTTY := openPromptTTY
	openPromptTTY = func() (io.ReadWriteCloser, error) { return tty, nil }
	t.Cleanup(func() { openPromptTTY = origOpenTTY })

	applyCalled := false
	origApplyHook := applyRestorePlanHook
	applyRestorePlanHook = func(restoreApplyContext) error {
		applyCalled = true
		return nil
	}
	t.Cleanup(func() { applyRestorePlanHook = origApplyHook })

	var stderr bytes.Buffer
	if err := runRestore(repoDir, []string{targetSHA}, &stderr); err != nil {
		t.Fatalf("runRestore() error = %v", err)
	}

	if applyCalled {
		t.Fatalf("restore apply should not run when user declines prompt")
	}
	if !strings.Contains(stderr.String(), "verti: restore skipped. Code and artifacts are now out of sync.") {
		t.Fatalf("expected out-of-sync warning in stderr, got %q", stderr.String())
	}

	afterContent, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read post-restore progress.md: %v", err)
	}
	if string(afterContent) != string(beforeContent) {
		t.Fatalf("declined restore should not modify artifacts")
	}
}

func TestRunRestoreAcceptProceedsToApplyRestorePlan(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-accept",
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

	origDecisionHook := beforeRestoreDecisionHook
	beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }
	t.Cleanup(func() { beforeRestoreDecisionHook = origDecisionHook })

	tty := newFakeTTY("y\n")
	origOpenTTY := openPromptTTY
	openPromptTTY = func() (io.ReadWriteCloser, error) { return tty, nil }
	t.Cleanup(func() { openPromptTTY = origOpenTTY })

	applyCalled := false
	origApplyHook := applyRestorePlanHook
	applyRestorePlanHook = func(ctx restoreApplyContext) error {
		applyCalled = true
		if len(ctx.Plan) == 0 {
			t.Fatalf("expected non-empty restore plan when applying accepted restore")
		}
		return nil
	}
	t.Cleanup(func() { applyRestorePlanHook = origApplyHook })

	var stderr bytes.Buffer
	if err := runRestore(repoDir, []string{targetSHA}, &stderr); err != nil {
		t.Fatalf("runRestore() error = %v", err)
	}
	if !applyCalled {
		t.Fatalf("restore apply should run when user accepts prompt")
	}
}

func TestRunRestoreNoTTYSkipsWithManualRecoveryHintAndNoFileChanges(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-no-tty-hint",
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

	beforeContent, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read pre-restore progress.md: %v", err)
	}

	origDecisionHook := beforeRestoreDecisionHook
	beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }
	t.Cleanup(func() { beforeRestoreDecisionHook = origDecisionHook })

	origOpenTTY := openPromptTTY
	openPromptTTY = func() (io.ReadWriteCloser, error) { return nil, fmt.Errorf("no tty") }
	t.Cleanup(func() { openPromptTTY = origOpenTTY })

	applyCalled := false
	origApplyHook := applyRestorePlanHook
	applyRestorePlanHook = func(restoreApplyContext) error {
		applyCalled = true
		return nil
	}
	t.Cleanup(func() { applyRestorePlanHook = origApplyHook })

	var stderr bytes.Buffer
	if err := runRestore(repoDir, []string{targetSHA}, &stderr); err != nil {
		t.Fatalf("runRestore() error = %v", err)
	}

	if applyCalled {
		t.Fatalf("restore apply should not run when tty is unavailable")
	}
	if !strings.Contains(stderr.String(), "verti restore "+targetSHA) {
		t.Fatalf("expected manual recovery hint in stderr, got %q", stderr.String())
	}

	afterContent, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read post-restore progress.md: %v", err)
	}
	if string(afterContent) != string(beforeContent) {
		t.Fatalf("no-tty restore skip should not modify artifacts")
	}
}

func TestRunRestoreForceAppliesWithoutPromptInInteractiveAndNonInteractiveModes(t *testing.T) {
	requireGit(t)

	setupAndRun := func(t *testing.T, name string, openTTY func() (io.ReadWriteCloser, error)) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			repoDir := createGitRepoWithArtifacts(t)
			storeRoot := filepath.Join(t.TempDir(), "store")
			cfg := config.Config{
				RepoID:        "repo-restore-force-" + strings.ReplaceAll(name, "_", "-"),
				Enabled:       true,
				Artifacts:     []string{"md", "progress.md"},
				StoreRoot:     storeRoot,
				RestoreMode:   config.RestoreModeForce,
				MaxFileSizeMB: config.DefaultMaxFileSizeMB,
			}
			writeRepoConfig(t, repoDir, cfg)

			var snapshotStderr bytes.Buffer
			if err := runSnapshot(repoDir, &snapshotStderr); err != nil {
				t.Fatalf("runSnapshot() error = %v", err)
			}
			targetSHA := runGit(t, repoDir, "rev-parse", "HEAD")

			origDecisionHook := beforeRestoreDecisionHook
			beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }
			t.Cleanup(func() { beforeRestoreDecisionHook = origDecisionHook })

			ttyOpened := false
			origOpenTTY := openPromptTTY
			openPromptTTY = func() (io.ReadWriteCloser, error) {
				ttyOpened = true
				return openTTY()
			}
			t.Cleanup(func() { openPromptTTY = origOpenTTY })

			promptCalled := false
			origPromptFn := promptRestoreConfirmationFn
			promptRestoreConfirmationFn = func(_ io.ReadWriter, _, _ string) (bool, error) {
				promptCalled = true
				return false, nil
			}
			t.Cleanup(func() { promptRestoreConfirmationFn = origPromptFn })

			applyCalled := false
			origApplyHook := applyRestorePlanHook
			applyRestorePlanHook = func(restoreApplyContext) error {
				applyCalled = true
				return nil
			}
			t.Cleanup(func() { applyRestorePlanHook = origApplyHook })

			var stderr bytes.Buffer
			if err := runRestore(repoDir, []string{targetSHA}, &stderr); err != nil {
				t.Fatalf("runRestore() error = %v", err)
			}

			if ttyOpened {
				t.Fatalf("force mode should not attempt to open tty")
			}
			if promptCalled {
				t.Fatalf("force mode should not call confirmation prompt")
			}
			if !applyCalled {
				t.Fatalf("force mode should apply restore without prompt")
			}
		})
	}

	setupAndRun(t, "interactive_tty_available", func() (io.ReadWriteCloser, error) {
		return newFakeTTY("n\n"), nil
	})
	setupAndRun(t, "non_interactive_no_tty", func() (io.ReadWriteCloser, error) {
		return nil, fmt.Errorf("no tty")
	})
}

func TestRunRestoreSkipModeExitsWithoutPromptOrApply(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-skip",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModeSkip,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var snapshotStderr bytes.Buffer
	if err := runSnapshot(repoDir, &snapshotStderr); err != nil {
		t.Fatalf("runSnapshot() error = %v", err)
	}
	targetSHA := runGit(t, repoDir, "rev-parse", "HEAD")

	origDecisionHook := beforeRestoreDecisionHook
	beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }
	t.Cleanup(func() { beforeRestoreDecisionHook = origDecisionHook })

	origOpenTTY := openPromptTTY
	openPromptTTY = func() (io.ReadWriteCloser, error) {
		t.Fatalf("skip mode should not attempt to open tty")
		return nil, nil
	}
	t.Cleanup(func() { openPromptTTY = origOpenTTY })

	promptCalled := false
	origPromptFn := promptRestoreConfirmationFn
	promptRestoreConfirmationFn = func(_ io.ReadWriter, _, _ string) (bool, error) {
		promptCalled = true
		return true, nil
	}
	t.Cleanup(func() { promptRestoreConfirmationFn = origPromptFn })

	applyCalled := false
	origApplyHook := applyRestorePlanHook
	applyRestorePlanHook = func(restoreApplyContext) error {
		applyCalled = true
		return nil
	}
	t.Cleanup(func() { applyRestorePlanHook = origApplyHook })

	var stderr bytes.Buffer
	if err := runRestore(repoDir, []string{targetSHA}, &stderr); err != nil {
		t.Fatalf("runRestore() error = %v", err)
	}

	if promptCalled {
		t.Fatalf("skip mode should not call confirmation prompt")
	}
	if applyCalled {
		t.Fatalf("skip mode should not apply restore")
	}
	if stderr.String() != "" {
		t.Fatalf("skip mode should exit silently, got stderr %q", stderr.String())
	}
}

func TestRunRestoreOrphanAppliesOrphanSnapshot(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-orphan-apply",
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

	entries, err := artifacts.BuildManifestEntries(repoDir, cfg.Artifacts)
	if err != nil {
		t.Fatalf("build manifest entries: %v", err)
	}

	scopeDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main")
	orphanID := "orphan-restore-1"
	if _, err := snapshots.PublishOrphanSnapshot(scopeDir, orphanID, entries, snapshots.Meta{
		WorktreeID:            "main",
		CreatedAt:             "2026-03-02T10:00:00Z",
		TriggeringCheckoutSHA: "checkout-123",
	}); err != nil {
		t.Fatalf("publish orphan snapshot: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("mutated progress\n"), 0o644); err != nil {
		t.Fatalf("mutate progress.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "md", "note.md"), []byte("mutated note\n"), 0o644); err != nil {
		t.Fatalf("mutate md/note.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "md", "stale.tmp"), []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	origOpenTTY := openPromptTTY
	openPromptTTY = func() (io.ReadWriteCloser, error) {
		t.Fatalf("orphan restore should not use interactive prompt")
		return nil, nil
	}
	t.Cleanup(func() { openPromptTTY = origOpenTTY })

	var restoreStderr bytes.Buffer
	if err := runRestore(repoDir, []string{"--orphan", orphanID}, &restoreStderr); err != nil {
		t.Fatalf("runRestore(--orphan) error = %v", err)
	}

	progressRaw, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read restored progress.md: %v", err)
	}
	if string(progressRaw) != "progress\n" {
		t.Fatalf("progress.md = %q, want %q", string(progressRaw), "progress\n")
	}

	noteRaw, err := os.ReadFile(filepath.Join(repoDir, "md", "note.md"))
	if err != nil {
		t.Fatalf("read restored md/note.md: %v", err)
	}
	if string(noteRaw) != "note\n" {
		t.Fatalf("md/note.md = %q, want %q", string(noteRaw), "note\n")
	}

	if _, err := os.Stat(filepath.Join(repoDir, "md", "stale.tmp")); !os.IsNotExist(err) {
		t.Fatalf("expected stale path removed after orphan restore, stat err=%v", err)
	}
}

func TestRunRestoreAppliesFilesDirsSymlinksAndQuarantinesStalePaths(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	if err := os.Symlink("progress.md", filepath.Join(repoDir, "current.link")); err != nil {
		t.Fatalf("create initial symlink: %v", err)
	}

	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-restore-apply",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md", "current.link"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModeForce,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var snapshotStderr bytes.Buffer
	if err := runSnapshot(repoDir, &snapshotStderr); err != nil {
		t.Fatalf("runSnapshot() error = %v", err)
	}
	targetSHA := runGit(t, repoDir, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("mutated progress\n"), 0o644); err != nil {
		t.Fatalf("mutate progress.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "md", "note.md"), []byte("mutated note\n"), 0o644); err != nil {
		t.Fatalf("mutate md/note.md: %v", err)
	}
	if err := os.Remove(filepath.Join(repoDir, "current.link")); err != nil {
		t.Fatalf("remove current.link: %v", err)
	}
	if err := os.Symlink("md/note.md", filepath.Join(repoDir, "current.link")); err != nil {
		t.Fatalf("mutate symlink target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "md", "stale.tmp"), []byte("stale-data\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	origDecisionHook := beforeRestoreDecisionHook
	beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }
	t.Cleanup(func() { beforeRestoreDecisionHook = origDecisionHook })

	origOpenTTY := openPromptTTY
	openPromptTTY = func() (io.ReadWriteCloser, error) {
		t.Fatalf("force mode should not open tty")
		return nil, nil
	}
	t.Cleanup(func() { openPromptTTY = origOpenTTY })

	var stderr bytes.Buffer
	if err := runRestore(repoDir, []string{targetSHA}, &stderr); err != nil {
		t.Fatalf("runRestore() error = %v", err)
	}

	progressRaw, err := os.ReadFile(filepath.Join(repoDir, "progress.md"))
	if err != nil {
		t.Fatalf("read progress.md after restore: %v", err)
	}
	if string(progressRaw) != "progress\n" {
		t.Fatalf("progress.md = %q, want %q", string(progressRaw), "progress\n")
	}

	noteRaw, err := os.ReadFile(filepath.Join(repoDir, "md", "note.md"))
	if err != nil {
		t.Fatalf("read md/note.md after restore: %v", err)
	}
	if string(noteRaw) != "note\n" {
		t.Fatalf("md/note.md = %q, want %q", string(noteRaw), "note\n")
	}

	linkTarget, err := os.Readlink(filepath.Join(repoDir, "current.link"))
	if err != nil {
		t.Fatalf("readlink current.link after restore: %v", err)
	}
	if linkTarget != "progress.md" {
		t.Fatalf("current.link target = %q, want %q", linkTarget, "progress.md")
	}

	if _, err := os.Stat(filepath.Join(repoDir, "md", "stale.tmp")); !os.IsNotExist(err) {
		t.Fatalf("expected stale path removed from repo after quarantine, stat err=%v", err)
	}

	quarantineRoot := filepath.Join(storeRoot, "repos", cfg.RepoID, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if err != nil {
		t.Fatalf("read quarantine root: %v", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("expected one quarantine session directory, got %#v", entries)
	}

	sessionDir := filepath.Join(quarantineRoot, entries[0].Name())
	metaRaw, err := os.ReadFile(filepath.Join(sessionDir, "meta.json"))
	if err != nil {
		t.Fatalf("read quarantine meta.json: %v", err)
	}

	var metaDoc map[string]any
	if err := json.Unmarshal(metaRaw, &metaDoc); err != nil {
		t.Fatalf("unmarshal quarantine meta.json: %v", err)
	}

	if metaDoc["repo_id"] != cfg.RepoID {
		t.Fatalf("quarantine repo_id = %v, want %q", metaDoc["repo_id"], cfg.RepoID)
	}
	if metaDoc["target_snapshot_sha"] != targetSHA {
		t.Fatalf("quarantine target_snapshot_sha = %v, want %q", metaDoc["target_snapshot_sha"], targetSHA)
	}

	quarantinedRaw, err := os.ReadFile(filepath.Join(sessionDir, "paths", "md", "stale.tmp"))
	if err != nil {
		t.Fatalf("read quarantined stale path: %v", err)
	}
	if string(quarantinedRaw) != "stale-data\n" {
		t.Fatalf("quarantined stale file = %q, want %q", string(quarantinedRaw), "stale-data\n")
	}
}

type fakeTTY struct {
	in     *bytes.Reader
	output bytes.Buffer
}

func newFakeTTY(input string) *fakeTTY {
	return &fakeTTY{
		in: bytes.NewReader([]byte(input)),
	}
}

func (f *fakeTTY) Read(p []byte) (int, error) {
	return f.in.Read(p)
}

func (f *fakeTTY) Write(p []byte) (int, error) {
	return f.output.Write(p)
}

func (f *fakeTTY) Close() error {
	return nil
}

func (f *fakeTTY) Output() string {
	return f.output.String()
}
