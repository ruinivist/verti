package verti

import (
	"testing"

	"verti/internal/testutil"
)

func TestSyncOutsideGitRepoFails(t *testing.T) {
	testutil.WithWorkingDir(t, t.TempDir(), func() {
		err := Sync()
		if err == nil {
			t.Fatal("Sync() error = nil, want error")
		}
		if err.Error() != "not a git repository" {
			t.Fatalf("Sync() error = %q, want %q", err.Error(), "not a git repository")
		}
	})
}

func TestOrphansOutsideGitRepoFails(t *testing.T) {
	testutil.WithWorkingDir(t, t.TempDir(), func() {
		err := Orphans()
		if err == nil {
			t.Fatal("Orphans() error = nil, want error")
		}
		if err.Error() != "not a git repository" {
			t.Fatalf("Orphans() error = %q, want %q", err.Error(), "not a git repository")
		}
	})
}

func TestRestoreOrphanOutsideGitRepoFails(t *testing.T) {
	testutil.WithWorkingDir(t, t.TempDir(), func() {
		err := RestoreOrphan(1)
		if err == nil {
			t.Fatal("RestoreOrphan() error = nil, want error")
		}
		if err.Error() != "not a git repository" {
			t.Fatalf("RestoreOrphan() error = %q, want %q", err.Error(), "not a git repository")
		}
	})
}
