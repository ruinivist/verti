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

func TestAddBootstrapsConfigAndHooksWithoutEditor(t *testing.T) {
	repoDir := testutil.NewRepo(t)
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

		config, err := os.ReadFile(filepath.Join(repoDir, configPath))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		if !bytes.HasPrefix(config, []byte(verticonfigManagedHeaderForTest())) {
			t.Fatalf("config missing managed header: %q", string(config))
		}

		referenceHook, err := os.ReadFile(filepath.Join(repoDir, referenceTransactionPath))
		if err != nil {
			t.Fatalf("read reference-transaction hook: %v", err)
		}
		if !strings.Contains(string(referenceHook), "\"/tmp/verti\" sync") {
			t.Fatalf("reference-transaction hook missing sync invocation: %q", string(referenceHook))
		}

		postCheckoutHook, err := os.ReadFile(filepath.Join(repoDir, postCheckoutHookPath))
		if err != nil {
			t.Fatalf("read post-checkout hook: %v", err)
		}
		if !strings.Contains(string(postCheckoutHook), "\"/tmp/verti\" sync") {
			t.Fatalf("post-checkout hook missing sync invocation: %q", string(postCheckoutHook))
		}

		exclude, err := os.ReadFile(filepath.Join(repoDir, excludePath))
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if string(exclude) != "docs\n" {
			t.Fatalf("exclude = %q, want %q", string(exclude), "docs\n")
		}
	})
}

func TestAddAppendsArtifactAndUpdatesExclude(t *testing.T) {
	repoDir := testutil.NewRepo(t)

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
		if string(exclude) != "# comment\nfoo\ndocs/guide.md\n" {
			t.Fatalf("exclude = %q, want %q", string(exclude), "# comment\nfoo\ndocs/guide.md\n")
		}
	})
}

func TestAddDuplicateArtifactIsNoOp(t *testing.T) {
	repoDir := testutil.NewRepo(t)

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

		if err := Add("/tmp/verti", "foo"); err != nil {
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
		if string(exclude) != "# comment\nfoo\n" {
			t.Fatalf("exclude = %q, want %q", string(exclude), "# comment\nfoo\n")
		}
	})
}

func TestAddRejectsInvalidArtifactPathBeforeBootstrap(t *testing.T) {
	repoDir := testutil.NewRepo(t)

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
