package verti

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	verticonfig "verti/internal/config"
	"verti/internal/testutil"
)

func TestInitCreatesConfigAndAppliesEmptyExcludeWithoutEditor(t *testing.T) {
	repoDir := testutil.NewRepo(t)

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := Init("/tmp/verti"); err != nil {
			t.Fatalf("Init() error = %v", err)
		}

		config, err := os.ReadFile(filepath.Join(repoDir, configPath))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}

		text := string(config)
		if !bytes.HasPrefix(config, []byte(verticonfigManagedHeaderForTest())) {
			t.Fatalf("config missing managed header: %q", text)
		}
		if !strings.Contains(text, "[verti]\n") {
			t.Fatalf("config missing table: %q", text)
		}
		if !strings.Contains(text, "repo_id = ") {
			t.Fatalf("config missing repo_id: %q", text)
		}
		if !strings.Contains(text, "artifacts = []\n") {
			t.Fatalf("config missing empty artifacts: %q", text)
		}

		cfg, err := verticonfig.ReadConfig(filepath.Join(repoDir, configPath))
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if cfg.RepoID == "" {
			t.Fatal("ReadConfig() RepoID = empty, want non-empty")
		}
		if len(cfg.Artifacts) != 0 {
			t.Fatalf("ReadConfig() Artifacts = %#v, want empty", cfg.Artifacts)
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
		if string(exclude) != managedExcludeBlockForTest() {
			t.Fatalf("exclude = %q, want %q", string(exclude), managedExcludeBlockForTest())
		}
	})
}

func TestInitPreservesExistingConfigAndRewritesHook(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	existingConfig := "[verti]\nrepo_id = \"existing\"\nartifacts = [\"foo\"]\n"

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
		wantExclude := "# comment\nfoo\n" + managedExcludeBlockForTest("foo")
		if string(gotExclude) != wantExclude {
			t.Fatalf("exclude changed = %q, want %q", string(gotExclude), wantExclude)
		}
	})
}

func TestInitIsIdempotentOnRepeat(t *testing.T) {
	repoDir := testutil.NewRepo(t)

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := Init("/tmp/verti"); err != nil {
			t.Fatalf("first Init() error = %v", err)
		}

		cfg, err := verticonfig.ReadConfig(configPath)
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		repoID := cfg.RepoID
		if repoID == "" {
			t.Fatal("ReadConfig() RepoID = empty, want non-empty")
		}

		if err := verticonfig.WriteConfig(configPath, verticonfig.Config{
			RepoID:    repoID,
			Artifacts: []string{"foo", "bar"},
		}); err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		if err := Init("/tmp/verti"); err != nil {
			t.Fatalf("second Init() error = %v", err)
		}

		exclude, err := os.ReadFile(excludePath)
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if string(exclude) != managedExcludeBlockForTest("foo", "bar") {
			t.Fatalf("exclude = %q, want %q", string(exclude), managedExcludeBlockForTest("foo", "bar"))
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
