package gitrepo

import (
	"testing"

	"verti/internal/testutil"
)

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
