package verti

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesConfigAndHook(t *testing.T) {
	repoDir := newTestRepo(t)

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
		if !strings.Contains(text, "artifacts = []\n") {
			t.Fatalf("config missing artifacts: %q", text)
		}
		if strings.Contains(text, "test.md") {
			t.Fatalf("config should not mention test.md: %q", text)
		}

		cfg, err := ReadConfig(filepath.Join(repoDir, configPath))
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if cfg.RepoID == "" {
			t.Fatal("ReadConfig() RepoID = empty, want non-empty")
		}
		if len(cfg.Artifacts) != 0 {
			t.Fatalf("ReadConfig() Artifacts = %#v, want empty", cfg.Artifacts)
		}

		hook, err := os.ReadFile(filepath.Join(repoDir, hookPath))
		if err != nil {
			t.Fatalf("read hook: %v", err)
		}
		if !strings.Contains(string(hook), "\"/tmp/verti\" sync") {
			t.Fatalf("hook missing sync invocation: %q", string(hook))
		}
	})
}

func TestInitPreservesExistingConfigAndRewritesHook(t *testing.T) {
	repoDir := newTestRepo(t)
	existingConfig := "[verti]\nrepo_id = \"existing\"\nartifacts = [\"foo\"]\n"

	withWorkingDir(t, repoDir, func() {
		if err := os.WriteFile(configPath, []byte(existingConfig), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := os.WriteFile(hookPath, []byte("old hook"), 0o755); err != nil {
			t.Fatalf("write hook: %v", err)
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
		t.Fatalf("mkdir repo: %v", err)
	}
	return dir
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
