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
