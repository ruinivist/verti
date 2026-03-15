package verti

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	verticonfig "verti/internal/config"
	"verti/internal/gitrepo"
	"verti/internal/testutil"
)

func TestAddBootstrapsConfigAndHooksWithoutEditor(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	configPath := testutil.VertiConfigPath(t, repoDir)
	referenceTransactionPath := testutil.ReferenceTransactionHookPath(t, repoDir)
	postCheckoutPath := testutil.PostCheckoutHookPath(t, repoDir)
	excludePath := testutil.ExcludePath(t, repoDir)
	t.Setenv("GIT_EDITOR", testutil.NewFakeEditor(t, repoDir, "#!/bin/sh\nexit 99\n"))
	t.Setenv("EDITOR", "")

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := Add("/tmp/verti", "docs"); err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		cfg, err := verticonfig.ReadConfig(configPath)
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if cfg.RepoID == "" {
			t.Fatal("ReadConfig() RepoID = empty, want non-empty")
		}
		if strings.Join(cfg.Artifacts, ",") != "docs" {
			t.Fatalf("ReadConfig() Artifacts = %#v, want %#v", cfg.Artifacts, []string{"docs"})
		}

		config, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		if !bytes.HasPrefix(config, []byte(verticonfigManagedHeaderForTest())) {
			t.Fatalf("config missing managed header: %q", string(config))
		}

		referenceHook, err := os.ReadFile(referenceTransactionPath)
		if err != nil {
			t.Fatalf("read reference-transaction hook: %v", err)
		}
		if !strings.Contains(string(referenceHook), "\"/tmp/verti\" sync") {
			t.Fatalf("reference-transaction hook missing sync invocation: %q", string(referenceHook))
		}

		postCheckoutHook, err := os.ReadFile(postCheckoutPath)
		if err != nil {
			t.Fatalf("read post-checkout hook: %v", err)
		}
		if !strings.Contains(string(postCheckoutHook), "\"/tmp/verti\" sync") {
			t.Fatalf("post-checkout hook missing sync invocation: %q", string(postCheckoutHook))
		}

		exclude, err := os.ReadFile(excludePath)
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if string(exclude) != managedExcludeBlockForTest("/docs") {
			t.Fatalf("exclude = %q, want %q", string(exclude), managedExcludeBlockForTest("/docs"))
		}

		managed, err := gitrepo.ReadManagedExcludes(excludePath)
		if err != nil {
			t.Fatalf("ReadManagedExcludes() error = %v", err)
		}
		if !reflect.DeepEqual(managed, []string{"/docs"}) {
			t.Fatalf("ReadManagedExcludes() = %#v, want %#v", managed, []string{"/docs"})
		}
	})
}

func TestAddAppendsArtifactAndUpdatesExclude(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	configPath := testutil.VertiConfigPath(t, repoDir)
	excludePath := testutil.ExcludePath(t, repoDir)

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := verticonfig.WriteConfig(configPath, verticonfig.Config{
			RepoID:    "repo-add",
			Artifacts: []string{"foo"},
		}); err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
		if err := os.WriteFile(excludePath, []byte("# comment\nfoo\n"), 0o644); err != nil {
			t.Fatalf("write exclude: %v", err)
		}

		if err := Add("/tmp/verti-new", "docs/guide.md"); err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		cfg, err := verticonfig.ReadConfig(configPath)
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if strings.Join(cfg.Artifacts, ",") != "foo,docs/guide.md" {
			t.Fatalf("ReadConfig() Artifacts = %#v, want %#v", cfg.Artifacts, []string{"foo", "docs/guide.md"})
		}

		exclude, err := os.ReadFile(excludePath)
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		wantExclude := "# comment\nfoo\n" + managedExcludeBlockForTest("/foo", "/docs/guide.md")
		if string(exclude) != wantExclude {
			t.Fatalf("exclude = %q, want %q", string(exclude), wantExclude)
		}
	})
}

func TestAddDuplicateArtifactIsNoOp(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	configPath := testutil.VertiConfigPath(t, repoDir)
	excludePath := testutil.ExcludePath(t, repoDir)

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := verticonfig.WriteConfig(configPath, verticonfig.Config{
			RepoID:    "repo-dup",
			Artifacts: []string{"foo"},
		}); err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
		if err := os.WriteFile(excludePath, []byte("# comment\nfoo\n"), 0o644); err != nil {
			t.Fatalf("write exclude: %v", err)
		}

		if err := Add("/tmp/verti", "/foo"); err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		cfg, err := verticonfig.ReadConfig(configPath)
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if strings.Join(cfg.Artifacts, ",") != "foo" {
			t.Fatalf("ReadConfig() Artifacts = %#v, want %#v", cfg.Artifacts, []string{"foo"})
		}

		exclude, err := os.ReadFile(excludePath)
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		wantExclude := "# comment\nfoo\n" + managedExcludeBlockForTest("/foo")
		if string(exclude) != wantExclude {
			t.Fatalf("exclude = %q, want %q", string(exclude), wantExclude)
		}
	})
}

func TestAddRootedAndUnrootedDirectoryArtifactsDeduplicate(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	configPath := testutil.VertiConfigPath(t, repoDir)

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := os.MkdirAll("docs", 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := verticonfig.WriteConfig(configPath, verticonfig.Config{
			RepoID:    "repo-dir-dup",
			Artifacts: []string{"docs/"},
		}); err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}

		if err := Add("/tmp/verti", "/docs/"); err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		cfg, err := verticonfig.ReadConfig(configPath)
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if strings.Join(cfg.Artifacts, ",") != "docs/" {
			t.Fatalf("ReadConfig() Artifacts = %#v, want %#v", cfg.Artifacts, []string{"docs/"})
		}
	})
}

func TestAddDirectoryOnlyArtifactRequiresDirectory(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	testutil.WriteFile(t, filepath.Join(repoDir, "notes.txt"), "notes\n")

	testutil.WithWorkingDir(t, repoDir, func() {
		err := Add("/tmp/verti", "/notes.txt/")
		if err == nil {
			t.Fatal("Add() error = nil, want error")
		}
		if err.Error() != "artifact is not a directory: notes.txt" {
			t.Fatalf("Add() error = %q, want %q", err.Error(), "artifact is not a directory: notes.txt")
		}
	})
}

func TestAddRejectsInvalidArtifactPathBeforeBootstrap(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	configPath := testutil.VertiConfigPath(t, repoDir)

	testutil.WithWorkingDir(t, repoDir, func() {
		err := Add("/tmp/verti", "../outside.txt")
		if err == nil {
			t.Fatal("Add() error = nil, want error")
		}
		if err.Error() != "invalid artifact path \"../outside.txt\": must not escape repository" {
			t.Fatalf("Add() error = %q, want %q", err.Error(), "invalid artifact path \"../outside.txt\": must not escape repository")
		}

		if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
			t.Fatalf("Stat(%q) error = %v, want not exist", configPath, statErr)
		}
	})
}

func verticonfigManagedHeaderForTest() string {
	return "# Managed by Verti. Manual edits may be rewritten.\n# Comments and formatting in this file will be lost.\n\n"
}

func managedExcludeBlockForTest(artifacts ...string) string {
	block := "# ===== verti start =====\n"
	for _, artifact := range artifacts {
		block += artifact + "\n"
	}
	block += "# ===== verti end =====\n"
	return block
}
