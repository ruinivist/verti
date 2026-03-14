package verti

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	verticonfig "verti/internal/config"
	"verti/internal/testutil"
)

func TestInitCreatesConfigAndHook(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	editor := testutil.NewFakeEditor(t, repoDir, "#!/bin/sh\ncat <<'EOF' > \"$1\"\n[verti]\nrepo_id = \"repo-1\"\nartifacts = [\"test.md\", \"docs\"]\nEOF\n")
	t.Setenv("GIT_EDITOR", editor)
	t.Setenv("EDITOR", "")

	testutil.WithWorkingDir(t, repoDir, func() {
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
		if !strings.Contains(text, "artifacts = [\"test.md\", \"docs\"]\n") {
			t.Fatalf("config missing edited artifacts: %q", text)
		}

		cfg, err := verticonfig.ReadConfig(filepath.Join(repoDir, configPath))
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if cfg.RepoID == "" {
			t.Fatal("ReadConfig() RepoID = empty, want non-empty")
		}
		if strings.Join(cfg.Artifacts, ",") != "test.md,docs" {
			t.Fatalf("ReadConfig() Artifacts = %#v, want %#v", cfg.Artifacts, []string{"test.md", "docs"})
		}

		hook, err := os.ReadFile(filepath.Join(repoDir, referenceTransactionPath))
		if err != nil {
			t.Fatalf("read reference-transaction hook: %v", err)
		}
		if !strings.Contains(string(hook), "\"/tmp/verti\" sync") {
			t.Fatalf("reference-transaction hook missing sync invocation: %q", string(hook))
		}

		postCheckoutHook, err := os.ReadFile(filepath.Join(repoDir, postCheckoutHookPath))
		if err != nil {
			t.Fatalf("read post-checkout hook: %v", err)
		}
		if !strings.Contains(string(postCheckoutHook), "\"/tmp/verti\" sync") {
			t.Fatalf("post-checkout hook missing sync invocation: %q", string(postCheckoutHook))
		}
		if !strings.Contains(string(postCheckoutHook), "if [ \"$old\" != \"$new\" ]; then") {
			t.Fatalf("post-checkout hook missing same-commit guard: %q", string(postCheckoutHook))
		}

		exclude, err := os.ReadFile(filepath.Join(repoDir, excludePath))
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if string(exclude) != "test.md\ndocs\n" {
			t.Fatalf("exclude = %q, want %q", string(exclude), "test.md\ndocs\n")
		}
	})
}

func TestInitPreservesExistingConfigAndRewritesHook(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	existingConfig := "[verti]\nrepo_id = \"existing\"\nartifacts = [\"foo\"]\n"
	editor := testutil.NewFakeEditor(t, repoDir, "#!/bin/sh\nexit 0\n")
	t.Setenv("GIT_EDITOR", editor)
	t.Setenv("EDITOR", "")

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := os.WriteFile(configPath, []byte(existingConfig), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := os.WriteFile(referenceTransactionPath, []byte("old hook"), 0o755); err != nil {
			t.Fatalf("write reference-transaction hook: %v", err)
		}
		if err := os.WriteFile(postCheckoutHookPath, []byte("old post-checkout hook"), 0o755); err != nil {
			t.Fatalf("write post-checkout hook: %v", err)
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

		gotHook, err := os.ReadFile(referenceTransactionPath)
		if err != nil {
			t.Fatalf("read reference-transaction hook: %v", err)
		}
		if !strings.Contains(string(gotHook), "\"/tmp/verti-new\" sync") {
			t.Fatalf("reference-transaction hook not rewritten: %q", string(gotHook))
		}

		gotPostCheckoutHook, err := os.ReadFile(postCheckoutHookPath)
		if err != nil {
			t.Fatalf("read post-checkout hook: %v", err)
		}
		if !strings.Contains(string(gotPostCheckoutHook), "\"/tmp/verti-new\" sync") {
			t.Fatalf("post-checkout hook not rewritten: %q", string(gotPostCheckoutHook))
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
	repoDir := testutil.NewRepo(t)
	firstEditor := testutil.NewFakeEditor(t, repoDir, "#!/bin/sh\ncat <<'EOF' > \"$1\"\n[verti]\nrepo_id = \"repo-repeat\"\nartifacts = [\"foo\", \"bar\"]\nEOF\n")
	secondEditor := testutil.NewFakeEditor(t, filepath.Join(repoDir, "second"), "#!/bin/sh\ncat <<'EOF' > \"$1\"\n[verti]\nrepo_id = \"repo-repeat\"\nartifacts = [\"foo\", \"bar\", \"baz\"]\nEOF\n")
	t.Setenv("EDITOR", "")

	testutil.WithWorkingDir(t, repoDir, func() {
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

	testutil.WithWorkingDir(t, dir, func() {
		err := Init("/tmp/verti")
		if err == nil {
			t.Fatal("Init() error = nil, want error")
		}
		if err.Error() != "not a git repository" {
			t.Fatalf("Init() error = %q, want %q", err.Error(), "not a git repository")
		}
	})
}
