package gitrepo

import (
	"path/filepath"
	"testing"

	"verti/internal/testutil"
)

func TestEnsureGitDir(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)

	testutil.WithWorkingDir(t, repoDir, func() {
		if err := EnsureGitDir(); err != nil {
			t.Fatalf("EnsureGitDir() error = %v", err)
		}
	})
}

func TestEnsureGitDirOutsideRepoFails(t *testing.T) {
	dir := t.TempDir()

	testutil.WithWorkingDir(t, dir, func() {
		if err := EnsureGitDir(); err == nil {
			t.Fatal("EnsureGitDir() error = nil, want error")
		}
	})
}

func TestResolvePaths(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)

	testutil.WithWorkingDir(t, repoDir, func() {
		got, err := ResolvePaths()
		if err != nil {
			t.Fatalf("ResolvePaths() error = %v", err)
		}

		wantCommonDir := testutil.GitCommonDir(t, repoDir)
		if got.CommonDir != wantCommonDir {
			t.Fatalf("ResolvePaths() CommonDir = %q, want %q", got.CommonDir, wantCommonDir)
		}
		if got.Config != filepath.Join(wantCommonDir, "verti.toml") {
			t.Fatalf("ResolvePaths() Config = %q, want %q", got.Config, filepath.Join(wantCommonDir, "verti.toml"))
		}
		if got.ReferenceTransactionHook != filepath.Join(wantCommonDir, "hooks", "reference-transaction") {
			t.Fatalf("ResolvePaths() ReferenceTransactionHook = %q, want %q", got.ReferenceTransactionHook, filepath.Join(wantCommonDir, "hooks", "reference-transaction"))
		}
		if got.PostCheckoutHook != filepath.Join(wantCommonDir, "hooks", "post-checkout") {
			t.Fatalf("ResolvePaths() PostCheckoutHook = %q, want %q", got.PostCheckoutHook, filepath.Join(wantCommonDir, "hooks", "post-checkout"))
		}
		if got.Exclude != filepath.Join(wantCommonDir, "info", "exclude") {
			t.Fatalf("ResolvePaths() Exclude = %q, want %q", got.Exclude, filepath.Join(wantCommonDir, "info", "exclude"))
		}
	})
}

func TestResolvePathsOutsideRepoFails(t *testing.T) {
	dir := t.TempDir()

	testutil.WithWorkingDir(t, dir, func() {
		if _, err := ResolvePaths(); err == nil {
			t.Fatal("ResolvePaths() error = nil, want error")
		}
	})
}

func TestHeadDisplay(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)

	testutil.WithWorkingDir(t, repoDir, func() {
		got, err := HeadDisplay()
		if err != nil {
			t.Fatalf("HeadDisplay() error = %v", err)
		}

		want := testutil.RunGit(t, repoDir, "show", "-s", "--format=%s [%h]", "HEAD")
		if got != want {
			t.Fatalf("HeadDisplay() = %q, want %q", got, want)
		}
	})
}
