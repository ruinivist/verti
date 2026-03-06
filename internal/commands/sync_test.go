package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"verti/internal/config"
	"verti/internal/snapshots"
)

func TestRunSyncMissingSnapshotPublishesCurrentStateSnapshot(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-sync-publish",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync() error = %v", err)
	}

	sha := runGit(t, repoDir, "rev-parse", "HEAD")
	snapshotID := mustSnapshotID(t, "main", sha)
	snapshotDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "snapshots", snapshotID)
	if _, err := os.Stat(filepath.Join(snapshotDir, "manifest.json")); err != nil {
		t.Fatalf("manifest.json not found in published snapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snapshotDir, "meta.json")); err != nil {
		t.Fatalf("meta.json not found in published snapshot: %v", err)
	}
}

func TestRunSyncExistingSnapshotRestoresInForceMode(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-sync-force-restore",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModeForce,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync(initial) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("mutate progress.md: %v", err)
	}

	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync(restore) error = %v", err)
	}

	got := mustReadFile(t, filepath.Join(repoDir, "progress.md"))
	if got != "progress\n" {
		t.Fatalf("expected restored artifact contents, got %q", got)
	}
}

func TestRunSyncDetachedHeadUsesDetachedBranchIdentity(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-sync-detached",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	runGit(t, repoDir, "checkout", "--detach", "HEAD")

	var stderr bytes.Buffer
	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync() error = %v", err)
	}

	sha := runGit(t, repoDir, "rev-parse", "HEAD")
	snapshotID := mustSnapshotID(t, snapshots.DetachedBranchIdentity, sha)
	snapshotDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "snapshots", snapshotID)
	metaRaw, err := os.ReadFile(filepath.Join(snapshotDir, "meta.json"))
	if err != nil {
		t.Fatalf("read meta.json: %v", err)
	}
	var meta snapshots.Meta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		t.Fatalf("unmarshal meta.json: %v", err)
	}
	if meta.Branch != snapshots.DetachedBranchIdentity {
		t.Fatalf("meta.branch = %q, want %q", meta.Branch, snapshots.DetachedBranchIdentity)
	}
}

func TestRunSyncPromptModeNoTTYSkipsRestoreWithManualHint(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-sync-no-tty",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync(initial) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("prompt-no-tty\n"), 0o644); err != nil {
		t.Fatalf("mutate progress.md: %v", err)
	}

	origOpenTTY := openPromptTTY
	openPromptTTY = func() (io.ReadWriteCloser, error) { return nil, os.ErrNotExist }
	t.Cleanup(func() { openPromptTTY = origOpenTTY })

	stderr.Reset()
	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync(no tty) error = %v", err)
	}

	if got := mustReadFile(t, filepath.Join(repoDir, "progress.md")); got != "prompt-no-tty\n" {
		t.Fatalf("prompt/no-tty should not alter artifacts, got %q", got)
	}
	if !strings.Contains(stderr.String(), "VERTI_RESTORE_MODE=force verti sync") {
		t.Fatalf("expected manual recovery hint in stderr, got %q", stderr.String())
	}
}

func TestRunSyncPromptModeDeclineSkipsRestore(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-sync-decline",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync(initial) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("declined\n"), 0o644); err != nil {
		t.Fatalf("mutate progress.md: %v", err)
	}

	origOpenTTY := openPromptTTY
	openPromptTTY = func() (io.ReadWriteCloser, error) { return newFakeTTY("N\n"), nil }
	t.Cleanup(func() { openPromptTTY = origOpenTTY })

	stderr.Reset()
	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync(decline) error = %v", err)
	}

	if got := mustReadFile(t, filepath.Join(repoDir, "progress.md")); got != "declined\n" {
		t.Fatalf("declined restore should not alter artifacts, got %q", got)
	}
	if !strings.Contains(stderr.String(), restoreSkippedOutOfSyncMessage) {
		t.Fatalf("expected out-of-sync warning, got %q", stderr.String())
	}
}

func TestRunSyncPromptModeAcceptRestores(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-sync-accept",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync(initial) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("accepted-mutation\n"), 0o644); err != nil {
		t.Fatalf("mutate progress.md: %v", err)
	}

	origOpenTTY := openPromptTTY
	openPromptTTY = func() (io.ReadWriteCloser, error) { return newFakeTTY("y\n"), nil }
	t.Cleanup(func() { openPromptTTY = origOpenTTY })

	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync(accept) error = %v", err)
	}

	if got := mustReadFile(t, filepath.Join(repoDir, "progress.md")); got != "progress\n" {
		t.Fatalf("accepted restore should restore artifacts, got %q", got)
	}
}

func TestRunSyncDebouncedCoalescesBurstToSingleApply(t *testing.T) {
	requireGit(t)

	repoDir := createGitRepoWithArtifacts(t)
	storeRoot := filepath.Join(t.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "repo-sync-debounce",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModeForce,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfig(t, repoDir, cfg)

	var stderr bytes.Buffer
	if err := runSync(repoDir, nil, &stderr); err != nil {
		t.Fatalf("runSync(initial) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("burst-mutation\n"), 0o644); err != nil {
		t.Fatalf("mutate progress.md: %v", err)
	}

	var applyCalls atomic.Int32
	origApply := applyRestorePlanHook
	applyRestorePlanHook = func(ctx restoreApplyContext) error {
		applyCalls.Add(1)
		return nil
	}
	t.Cleanup(func() { applyRestorePlanHook = origApply })

	var wg sync.WaitGroup
	errCh := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runSync(repoDir, []string{"--debounced"}, io.Discard)
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("runSync(--debounced) error = %v", err)
		}
	}

	if got := applyCalls.Load(); got != 1 {
		t.Fatalf("expected single restore apply after debounced burst, got %d", got)
	}
}

type fakeTTY struct {
	reader *strings.Reader
	output bytes.Buffer
}

func newFakeTTY(input string) *fakeTTY {
	return &fakeTTY{reader: strings.NewReader(input)}
}

func (f *fakeTTY) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

func (f *fakeTTY) Write(p []byte) (int, error) {
	return f.output.Write(p)
}

func (f *fakeTTY) Close() error { return nil }
