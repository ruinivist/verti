package verti

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	verticonfig "verti/internal/config"
	"verti/internal/testutil"
)

func TestRemoveDeletesArtifactAndUpdatesExclude(t *testing.T) {
	repoDir := testutil.NewRepo(t)

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := verticonfig.WriteConfig(configPath, verticonfig.Config{
			RepoID:    "repo-rm",
			Artifacts: []string{"foo", "docs/guide.md"},
		}); err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
		if err := os.WriteFile(excludePath, []byte("# comment\nfoo\n"+managedExcludeBlockForTest("/foo", "/docs/guide.md")), 0o644); err != nil {
			t.Fatalf("write exclude: %v", err)
		}

		if err := Remove("/tmp/verti", "/docs/guide.md"); err != nil {
			t.Fatalf("Remove() error = %v", err)
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

func TestRemoveUsesManagedExcludeBlockForNoOp(t *testing.T) {
	repoDir := testutil.NewRepo(t)

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := verticonfig.WriteConfig(configPath, verticonfig.Config{
			RepoID:    "repo-rm-noop",
			Artifacts: []string{"foo"},
		}); err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
		if err := os.WriteFile(excludePath, []byte("# comment\nmanual\n"), 0o644); err != nil {
			t.Fatalf("write exclude: %v", err)
		}

		if err := Remove("/tmp/verti", "/foo"); err != nil {
			t.Fatalf("Remove() error = %v", err)
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
		if string(exclude) != "# comment\nmanual\n" {
			t.Fatalf("exclude = %q, want %q", string(exclude), "# comment\nmanual\n")
		}
	})
}

func TestRemovePreservesTrailingSlashSemanticsWithoutFilesystemChecks(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	testutil.WriteFile(t, filepath.Join(repoDir, "notes.txt"), "notes\n")

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := verticonfig.WriteConfig(configPath, verticonfig.Config{
			RepoID:    "repo-rm-dir-spec",
			Artifacts: []string{"notes.txt/"},
		}); err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
		if err := os.WriteFile(excludePath, []byte(managedExcludeBlockForTest("/notes.txt/")), 0o644); err != nil {
			t.Fatalf("write exclude: %v", err)
		}

		if err := Remove("/tmp/verti", "/notes.txt/"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}

		cfg, err := verticonfig.ReadConfig(configPath)
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if len(cfg.Artifacts) != 0 {
			t.Fatalf("ReadConfig() Artifacts = %#v, want empty", cfg.Artifacts)
		}
	})
}

func TestRemoveRequiresExactTrailingSlashMatch(t *testing.T) {
	repoDir := testutil.NewRepo(t)

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := verticonfig.WriteConfig(configPath, verticonfig.Config{
			RepoID:    "repo-rm-exact",
			Artifacts: []string{"foo"},
		}); err != nil {
			t.Fatalf("WriteConfig() error = %v", err)
		}
		if err := os.WriteFile(excludePath, []byte(managedExcludeBlockForTest("/foo")), 0o644); err != nil {
			t.Fatalf("write exclude: %v", err)
		}

		if err := Remove("/tmp/verti", "foo/"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}

		cfg, err := verticonfig.ReadConfig(configPath)
		if err != nil {
			t.Fatalf("ReadConfig() error = %v", err)
		}
		if strings.Join(cfg.Artifacts, ",") != "foo" {
			t.Fatalf("ReadConfig() Artifacts = %#v, want %#v", cfg.Artifacts, []string{"foo"})
		}
	})
}

func TestRemoveRejectsInvalidArtifactPathBeforeBootstrap(t *testing.T) {
	repoDir := testutil.NewRepo(t)

	testutil.WithWorkingDir(t, repoDir, func() {
		err := Remove("/tmp/verti", "../outside.txt")
		if err == nil {
			t.Fatal("Remove() error = nil, want error")
		}
		if err.Error() != "invalid artifact path \"../outside.txt\": must not escape repository" {
			t.Fatalf("Remove() error = %q, want %q", err.Error(), "invalid artifact path \"../outside.txt\": must not escape repository")
		}

		if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
			t.Fatalf("Stat(%q) error = %v, want not exist", configPath, statErr)
		}
	})
}
