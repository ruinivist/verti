package gitrepo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWriteManagedExcludesCreatesBlock(t *testing.T) {
	tests := []struct {
		name      string
		artifacts []string
		want      string
	}{
		{
			name:      "empty block",
			artifacts: []string{},
			want:      managedExcludeBlockForTest(),
		},
		{
			name:      "non-empty block",
			artifacts: []string{"foo", "dir/baz"},
			want:      managedExcludeBlockForTest("/foo", "/dir/baz"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "info", "exclude")

			if err := WriteManagedExcludes(path, tt.artifacts); err != nil {
				t.Fatalf("WriteManagedExcludes() error = %v", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read exclude: %v", err)
			}

			if string(got) != tt.want {
				t.Fatalf("exclude = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestWriteManagedExcludesAppendsManagedBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exclude")
	if err := os.WriteFile(path, []byte("# comment\nfoo"), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	if err := WriteManagedExcludes(path, []string{"bar", "dir/baz"}); err != nil {
		t.Fatalf("WriteManagedExcludes() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}

	want := "# comment\nfoo\n" + managedExcludeBlockForTest("/bar", "/dir/baz")
	if string(got) != want {
		t.Fatalf("exclude = %q, want %q", string(got), want)
	}
}

func TestWriteManagedExcludesRewritesExistingBlockOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exclude")
	initial := "# comment\n" + managedExcludeBlockForTest("/old", "/entry") + "\n# suffix\nmanual\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	if err := WriteManagedExcludes(path, []string{"fresh"}); err != nil {
		t.Fatalf("WriteManagedExcludes() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}

	want := "# comment\n\n# suffix\nmanual\n" + managedExcludeBlockForTest("/fresh")
	if string(got) != want {
		t.Fatalf("exclude = %q, want %q", string(got), want)
	}
}

func TestReadManagedExcludesReturnsOnlyManagedPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exclude")
	content := "# comment\nfoo\n" + managedExcludeBlockForTest("/docs", "/notes.txt") + "manual\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	got, err := ReadManagedExcludes(path)
	if err != nil {
		t.Fatalf("ReadManagedExcludes() error = %v", err)
	}

	want := []string{"/docs", "/notes.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadManagedExcludes() = %#v, want %#v", got, want)
	}
}

func TestWriteManagedExcludesIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exclude")

	if err := WriteManagedExcludes(path, []string{"docs"}); err != nil {
		t.Fatalf("first WriteManagedExcludes() error = %v", err)
	}

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read first exclude: %v", err)
	}

	if err := WriteManagedExcludes(path, []string{"docs"}); err != nil {
		t.Fatalf("second WriteManagedExcludes() error = %v", err)
	}

	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read second exclude: %v", err)
	}

	if string(second) != string(first) {
		t.Fatalf("second exclude = %q, want %q", string(second), string(first))
	}
}

func managedExcludeBlockForTest(artifacts ...string) string {
	block := managedExcludeStart + "\n"
	for _, artifact := range artifacts {
		block += artifact + "\n"
	}
	block += managedExcludeEnd + "\n"
	return block
}
