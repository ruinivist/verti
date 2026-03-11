package gitrepo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendMissingExcludes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exclude")
	if err := os.WriteFile(path, []byte("# comment\nfoo\n"), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	if err := ExcludeArtifacts(path, []string{"foo", "bar", "dir/baz"}); err != nil {
		t.Fatalf("ExcludeArtifacts() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}

	want := "# comment\nfoo\nbar\ndir/baz\n"
	if string(got) != want {
		t.Fatalf("exclude = %q, want %q", string(got), want)
	}
}
