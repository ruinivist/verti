package verti

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesConfigAndHook(t *testing.T) {
	repoDir := newTestRepo(t)
	editor := newFakeEditor(t, repoDir, "#!/bin/sh\ncat <<'EOF' > \"$1\"\n[verti]\nrepo_id = \"repo-1\"\nartifacts = [\"test.md\", \"docs/output\"]\nEOF\n")
	t.Setenv("GIT_EDITOR", editor)
	t.Setenv("EDITOR", "")

	withWorkingDir(t, repoDir, func() {
		if err := Init("/tmp/verti"); err != nil {
			t.Fatalf("Init() error = %v", err)
		}

		config, err := os.ReadFile(filepath.Join(repoDir, configPath))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}

		text := string(config)
		if !strings.Contains(text, "[verti]\n") {
			t.Fatalf("config missing table: %q", text)
		}
		if !strings.Contains(text, "repo_id = ") {
			t.Fatalf("config missing repo_id: %q", text)
		}
		if !strings.Contains(text, "artifacts = [\"test.md\", \"docs/output\"]\n") {
			t.Fatalf("config missing edited artifacts: %q", text)
		}

		cfg, err := ReadConfig(filepath.Join(repoDir, configPath))
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if cfg.RepoID == "" {
			t.Fatal("ReadConfig() RepoID = empty, want non-empty")
		}
		if strings.Join(cfg.Artifacts, ",") != "test.md,docs/output" {
			t.Fatalf("ReadConfig() Artifacts = %#v, want %#v", cfg.Artifacts, []string{"test.md", "docs/output"})
		}

		hook, err := os.ReadFile(filepath.Join(repoDir, hookPath))
		if err != nil {
			t.Fatalf("read hook: %v", err)
		}
		if !strings.Contains(string(hook), "\"/tmp/verti\" sync") {
			t.Fatalf("hook missing sync invocation: %q", string(hook))
		}

		exclude, err := os.ReadFile(filepath.Join(repoDir, excludePath))
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if string(exclude) != "test.md\ndocs/output\n" {
			t.Fatalf("exclude = %q, want %q", string(exclude), "test.md\ndocs/output\n")
		}
	})
}

func TestInitPreservesExistingConfigAndRewritesHook(t *testing.T) {
	repoDir := newTestRepo(t)
	existingConfig := "[verti]\nrepo_id = \"existing\"\nartifacts = [\"foo\"]\n"
	editor := newFakeEditor(t, repoDir, "#!/bin/sh\nexit 0\n")
	t.Setenv("GIT_EDITOR", editor)
	t.Setenv("EDITOR", "")

	withWorkingDir(t, repoDir, func() {
		if err := os.WriteFile(configPath, []byte(existingConfig), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := os.WriteFile(hookPath, []byte("old hook"), 0o755); err != nil {
			t.Fatalf("write hook: %v", err)
		}
		if err := os.WriteFile(excludePath, []byte("# comment\nfoo\n"), 0o644); err != nil {
			t.Fatalf("write exclude: %v", err)
		}

		if err := Init("/tmp/verti-new"); err != nil {
			t.Fatalf("Init() error = %v", err)
		}

		gotConfig, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		if string(gotConfig) != existingConfig {
			t.Fatalf("config changed = %q, want %q", string(gotConfig), existingConfig)
		}

		gotHook, err := os.ReadFile(hookPath)
		if err != nil {
			t.Fatalf("read hook: %v", err)
		}
		if !strings.Contains(string(gotHook), "\"/tmp/verti-new\" sync") {
			t.Fatalf("hook not rewritten: %q", string(gotHook))
		}

		gotExclude, err := os.ReadFile(excludePath)
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if string(gotExclude) != "# comment\nfoo\n" {
			t.Fatalf("exclude changed = %q, want %q", string(gotExclude), "# comment\nfoo\n")
		}
	})
}

func TestInitAppendsOnlyNewArtifactsOnRepeat(t *testing.T) {
	repoDir := newTestRepo(t)
	firstEditor := newFakeEditor(t, repoDir, "#!/bin/sh\ncat <<'EOF' > \"$1\"\n[verti]\nrepo_id = \"repo-repeat\"\nartifacts = [\"foo\", \"bar\"]\nEOF\n")
	secondEditor := newFakeEditor(t, filepath.Join(repoDir, "second"), "#!/bin/sh\ncat <<'EOF' > \"$1\"\n[verti]\nrepo_id = \"repo-repeat\"\nartifacts = [\"foo\", \"bar\", \"baz\"]\nEOF\n")
	t.Setenv("EDITOR", "")

	withWorkingDir(t, repoDir, func() {
		t.Setenv("GIT_EDITOR", firstEditor)
		if err := Init("/tmp/verti"); err != nil {
			t.Fatalf("first Init() error = %v", err)
		}

		t.Setenv("GIT_EDITOR", secondEditor)
		if err := Init("/tmp/verti"); err != nil {
			t.Fatalf("second Init() error = %v", err)
		}

		exclude, err := os.ReadFile(excludePath)
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if string(exclude) != "foo\nbar\nbaz\n" {
			t.Fatalf("exclude = %q, want %q", string(exclude), "foo\nbar\nbaz\n")
		}
	})
}

func TestInitOutsideGitRepoFails(t *testing.T) {
	dir := t.TempDir()

	withWorkingDir(t, dir, func() {
		err := Init("/tmp/verti")
		if err == nil {
			t.Fatal("Init() error = nil, want error")
		}
		if err.Error() != "not a git repository" {
			t.Fatalf("Init() error = %q, want %q", err.Error(), "not a git repository")
		}
	})
}

func newTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir repo hooks: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	return dir
}

func newFakeEditor(t *testing.T, dir, content string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fake editor dir: %v", err)
	}
	path := filepath.Join(dir, "fake-editor")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake editor: %v", err)
	}
	return path
}

func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore dir: %v", err)
		}
	}()

	fn()
}
