package commands

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"verti/internal/config"
	"verti/internal/snapshots"
)

func TestMVPAcceptanceMatrixAC1ToAC9(t *testing.T) {
	requireGit(t)

	t.Run("AC1_init_idempotent", func(t *testing.T) {
		repoDir := createGitRepo(t)

		if err := runInit(repoDir, "/abs/path/to/verti"); err != nil {
			t.Fatalf("runInit(first) error = %v", err)
		}
		cfgPath := filepath.Join(repoDir, ".git", "verti.toml")
		first, err := config.Load(cfgPath)
		if err != nil {
			t.Fatalf("load first config: %v", err)
		}

		if err := runInit(repoDir, "/abs/path/to/verti"); err != nil {
			t.Fatalf("runInit(second) error = %v", err)
		}
		second, err := config.Load(cfgPath)
		if err != nil {
			t.Fatalf("load second config: %v", err)
		}

		if first.RepoID == "" || second.RepoID != first.RepoID {
			t.Fatalf("repo_id must be stable across init reruns: first=%q second=%q", first.RepoID, second.RepoID)
		}
	})

	t.Run("AC2_snapshot_writes_configured_artifacts", func(t *testing.T) {
		repoDir := createGitRepoWithArtifacts(t)
		storeRoot := filepath.Join(t.TempDir(), "store")
		cfg := config.Config{
			RepoID:        "ac2-repo",
			Enabled:       true,
			Artifacts:     []string{"md", "progress.md"},
			StoreRoot:     storeRoot,
			RestoreMode:   config.RestoreModePrompt,
			MaxFileSizeMB: config.DefaultMaxFileSizeMB,
		}
		writeRepoConfig(t, repoDir, cfg)

		var stderr bytes.Buffer
		if err := runSnapshot(repoDir, &stderr); err != nil {
			t.Fatalf("runSnapshot() error = %v", err)
		}
		sha := runGit(t, repoDir, "rev-parse", "HEAD")
		manifestPath := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "snapshots", sha, "manifest.json")
		entries := readManifestEntries(t, manifestPath)

		if entries["progress.md"].Status != "present" || entries["md/note.md"].Status != "present" {
			t.Fatalf("expected configured artifacts in snapshot manifest, got %#v", entries)
		}
	})

	t.Run("AC3_checkout_hook_restores_only_on_commit_change", func(t *testing.T) {
		repoDir := createGitRepo(t)
		foreignHook := "#!/usr/bin/env bash\nexit 0\n"
		hookPath := filepath.Join(repoDir, ".git", "hooks", "post-checkout")
		if err := os.WriteFile(hookPath, []byte(foreignHook), 0o755); err != nil {
			t.Fatalf("write foreign post-checkout hook: %v", err)
		}

		logPath := filepath.Join(t.TempDir(), "verti-hook.log")
		fakeVerti := filepath.Join(t.TempDir(), "verti-fake.sh")
		script := "#!/usr/bin/env bash\nset -euo pipefail\necho \"$*\" >> \"" + logPath + "\"\n"
		if err := os.WriteFile(fakeVerti, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake verti script: %v", err)
		}

		if err := runInit(repoDir, fakeVerti); err != nil {
			t.Fatalf("runInit() error = %v", err)
		}

		runHook(t, repoDir, hookPath, "a", "a", "1") // same commit
		runHook(t, repoDir, hookPath, "a", "b", "0") // non-commit checkout
		runHook(t, repoDir, hookPath, "a", "c", "1") // commit-changing checkout

		raw, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read hook log: %v", err)
		}
		lines := strings.Fields(strings.TrimSpace(string(raw)))
		got := strings.Join(lines, " ")
		if got != "restore c" {
			t.Fatalf("expected exactly one restore invocation for commit-changing checkout, got %q", got)
		}
	})

	t.Run("AC4_prompt_decline_no_changes_and_orphan_recorded", func(t *testing.T) {
		repoDir := createGitRepoWithArtifacts(t)
		storeRoot := filepath.Join(t.TempDir(), "store")
		cfg := config.Config{
			RepoID:        "ac4-repo",
			Enabled:       true,
			Artifacts:     []string{"md", "progress.md"},
			StoreRoot:     storeRoot,
			RestoreMode:   config.RestoreModePrompt,
			MaxFileSizeMB: config.DefaultMaxFileSizeMB,
		}
		writeRepoConfig(t, repoDir, cfg)
		var stderr bytes.Buffer
		if err := runSnapshot(repoDir, &stderr); err != nil {
			t.Fatalf("runSnapshot() error = %v", err)
		}
		sha := runGit(t, repoDir, "rev-parse", "HEAD")

		if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("changed\n"), 0o644); err != nil {
			t.Fatalf("mutate progress.md: %v", err)
		}
		before := mustReadFile(t, filepath.Join(repoDir, "progress.md"))

		origOpenTTY := openPromptTTY
		openPromptTTY = func() (io.ReadWriteCloser, error) { return newFakeTTY("N\n"), nil }
		t.Cleanup(func() { openPromptTTY = origOpenTTY })

		var restoreStderr bytes.Buffer
		if err := runRestore(repoDir, []string{sha}, &restoreStderr); err != nil {
			t.Fatalf("runRestore() error = %v", err)
		}
		after := mustReadFile(t, filepath.Join(repoDir, "progress.md"))
		if after != before {
			t.Fatalf("declined restore must keep artifact content unchanged")
		}

		orphansDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main", "orphans")
		dirEntries, err := os.ReadDir(orphansDir)
		if err != nil || len(dirEntries) == 0 {
			t.Fatalf("expected orphan snapshot to be recorded before prompt, err=%v entries=%d", err, len(dirEntries))
		}
	})

	t.Run("AC5_force_mode_applies_with_or_without_tty", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			tty  func() (io.ReadWriteCloser, error)
		}{
			{name: "interactive", tty: func() (io.ReadWriteCloser, error) { return newFakeTTY(""), nil }},
			{name: "non_interactive", tty: func() (io.ReadWriteCloser, error) { return nil, os.ErrNotExist }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				repoDir := createGitRepoWithArtifacts(t)
				storeRoot := filepath.Join(t.TempDir(), "store")
				cfg := config.Config{
					RepoID:        "ac5-repo-" + tc.name,
					Enabled:       true,
					Artifacts:     []string{"md", "progress.md"},
					StoreRoot:     storeRoot,
					RestoreMode:   config.RestoreModeForce,
					MaxFileSizeMB: config.DefaultMaxFileSizeMB,
				}
				writeRepoConfig(t, repoDir, cfg)
				var stderr bytes.Buffer
				if err := runSnapshot(repoDir, &stderr); err != nil {
					t.Fatalf("runSnapshot() error = %v", err)
				}
				sha := runGit(t, repoDir, "rev-parse", "HEAD")

				if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("force-mutation\n"), 0o644); err != nil {
					t.Fatalf("mutate progress.md: %v", err)
				}

				origOpenTTY := openPromptTTY
				openPromptTTY = tc.tty
				t.Cleanup(func() { openPromptTTY = origOpenTTY })

				var restoreStderr bytes.Buffer
				if err := runRestore(repoDir, []string{sha}, &restoreStderr); err != nil {
					t.Fatalf("runRestore() error = %v", err)
				}
				got := mustReadFile(t, filepath.Join(repoDir, "progress.md"))
				if got != "progress\n" {
					t.Fatalf("force mode should apply restore; got %q", got)
				}
			})
		}
	})

	t.Run("AC6_skip_mode_silent_no_restore", func(t *testing.T) {
		repoDir := createGitRepoWithArtifacts(t)
		storeRoot := filepath.Join(t.TempDir(), "store")
		cfg := config.Config{
			RepoID:        "ac6-repo",
			Enabled:       true,
			Artifacts:     []string{"md", "progress.md"},
			StoreRoot:     storeRoot,
			RestoreMode:   config.RestoreModeSkip,
			MaxFileSizeMB: config.DefaultMaxFileSizeMB,
		}
		writeRepoConfig(t, repoDir, cfg)
		var stderr bytes.Buffer
		if err := runSnapshot(repoDir, &stderr); err != nil {
			t.Fatalf("runSnapshot() error = %v", err)
		}
		sha := runGit(t, repoDir, "rev-parse", "HEAD")
		if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("skip-mode\n"), 0o644); err != nil {
			t.Fatalf("mutate progress.md: %v", err)
		}

		var restoreStderr bytes.Buffer
		if err := runRestore(repoDir, []string{sha}, &restoreStderr); err != nil {
			t.Fatalf("runRestore() error = %v", err)
		}
		if restoreStderr.String() != "" {
			t.Fatalf("skip mode should be silent, got stderr %q", restoreStderr.String())
		}
		if got := mustReadFile(t, filepath.Join(repoDir, "progress.md")); got != "skip-mode\n" {
			t.Fatalf("skip mode should not alter artifacts, got %q", got)
		}
	})

	t.Run("AC7_prompt_no_tty_manual_hint", func(t *testing.T) {
		repoDir := createGitRepoWithArtifacts(t)
		storeRoot := filepath.Join(t.TempDir(), "store")
		cfg := config.Config{
			RepoID:        "ac7-repo",
			Enabled:       true,
			Artifacts:     []string{"md", "progress.md"},
			StoreRoot:     storeRoot,
			RestoreMode:   config.RestoreModePrompt,
			MaxFileSizeMB: config.DefaultMaxFileSizeMB,
		}
		writeRepoConfig(t, repoDir, cfg)
		var stderr bytes.Buffer
		if err := runSnapshot(repoDir, &stderr); err != nil {
			t.Fatalf("runSnapshot() error = %v", err)
		}
		sha := runGit(t, repoDir, "rev-parse", "HEAD")

		origOpenTTY := openPromptTTY
		openPromptTTY = func() (io.ReadWriteCloser, error) { return nil, os.ErrNotExist }
		t.Cleanup(func() { openPromptTTY = origOpenTTY })

		var restoreStderr bytes.Buffer
		if err := runRestore(repoDir, []string{sha}, &restoreStderr); err != nil {
			t.Fatalf("runRestore() error = %v", err)
		}
		if !strings.Contains(restoreStderr.String(), "verti restore "+sha) {
			t.Fatalf("expected manual recovery hint in stderr, got %q", restoreStderr.String())
		}
	})

	t.Run("AC8_hooks_preserve_versioned_backups", func(t *testing.T) {
		repoDir := createGitRepo(t)
		hookPath := filepath.Join(repoDir, ".git", "hooks", "post-merge")
		first := "#!/usr/bin/env bash\necho first\n"
		if err := os.WriteFile(hookPath, []byte(first), 0o755); err != nil {
			t.Fatalf("write first foreign hook: %v", err)
		}

		if err := runInit(repoDir, "/abs/path/to/verti"); err != nil {
			t.Fatalf("runInit(first) error = %v", err)
		}

		second := "#!/usr/bin/env bash\necho second\n"
		if err := os.WriteFile(hookPath, []byte(second), 0o755); err != nil {
			t.Fatalf("write second foreign hook: %v", err)
		}
		if err := runInit(repoDir, "/abs/path/to/verti"); err != nil {
			t.Fatalf("runInit(second) error = %v", err)
		}

			backup0 := mustReadFile(t, hookPath+".verti.orig-hooks")
			backup1 := mustReadFile(t, hookPath+".verti.orig-hooks1")
		dispatcher := mustReadFile(t, hookPath)
		if backup0 != first || backup1 != second {
			t.Fatalf("unexpected backup slot contents")
		}
			if !strings.Contains(dispatcher, "LEGACY_HOOK=\""+hookPath+".verti.orig-hooks1\"") {
				t.Fatalf("dispatcher should execute latest backup slot only")
			}
	})

	t.Run("AC9_list_formatted_with_orphans_and_worktree_isolation", func(t *testing.T) {
		repoDir := createGitRepoWithArtifacts(t)
		storeRoot := filepath.Join(t.TempDir(), "store")
		cfg := config.Config{
			RepoID:        "ac9-repo",
			Enabled:       true,
			Artifacts:     []string{"md", "progress.md"},
			StoreRoot:     storeRoot,
			RestoreMode:   config.RestoreModePrompt,
			MaxFileSizeMB: config.DefaultMaxFileSizeMB,
		}
		writeRepoConfig(t, repoDir, cfg)

		var stderr bytes.Buffer
		if err := runSnapshot(repoDir, &stderr); err != nil {
			t.Fatalf("runSnapshot(main) error = %v", err)
		}

		worktreeDir := filepath.Join(t.TempDir(), "feature-worktree")
		runGit(t, repoDir, "worktree", "add", "-b", "feature/ac9", worktreeDir)
		t.Cleanup(func() { runGit(t, repoDir, "worktree", "remove", "--force", worktreeDir) })

		if err := os.WriteFile(filepath.Join(worktreeDir, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
			t.Fatalf("write feature worktree file: %v", err)
		}
		runGit(t, worktreeDir, "add", "feature.txt")
		runGit(t, worktreeDir, "commit", "-m", "feature commit")

		if err := runSnapshot(worktreeDir, &stderr); err != nil {
			t.Fatalf("runSnapshot(worktree) error = %v", err)
		}

		mainScope := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", "main")
		if _, err := snapshots.PublishOrphanSnapshot(mainScope, "ac9-orphan", nil, snapshots.Meta{
			WorktreeID:            "main",
			CreatedAt:             "2026-03-02T12:00:00Z",
			TriggeringCheckoutSHA: "checkout-ac9",
		}); err != nil {
			t.Fatalf("publish orphan snapshot: %v", err)
		}

		var listStdout bytes.Buffer
		if err := runList(repoDir, []string{"--orphans"}, &listStdout); err != nil {
			t.Fatalf("runList(--orphans) error = %v", err)
		}
		out := listStdout.String()
		if !strings.Contains(out, "COMMIT") || !strings.Contains(out, "ORPHAN_ID") {
			t.Fatalf("expected formatted list columns, got %q", out)
		}
		if !strings.Contains(out, "ac9-orphan") {
			t.Fatalf("expected orphan row in list --orphans output, got %q", out)
		}

		featureSHA := runGit(t, worktreeDir, "rev-parse", "HEAD")
		if strings.Contains(out, featureSHA) {
			t.Fatalf("main-worktree list output should not include linked-worktree snapshot %q", featureSHA)
		}
	})
}

func runHook(t *testing.T, dir, hookPath string, args ...string) {
	t.Helper()
	cmd := exec.Command(hookPath, args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("run hook %q args=%v: %v stderr=%q", hookPath, args, err, stderr.String())
	}
}
