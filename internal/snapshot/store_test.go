package snapshot

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreEnsureDirsCreatesOrphanStore(t *testing.T) {
	s := newStore(t.TempDir(), "repo-store")

	if err := s.ensureDirs(); err != nil {
		t.Fatalf("ensureDirs() error = %v", err)
	}

	for _, path := range []string{s.root, s.blobsPath(), s.manifestsPath(), s.orphansPath()} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("path %q is not a directory", path)
		}
	}
}

func TestStoreOrphanManifestPathsReturnsSortedJSONFiles(t *testing.T) {
	s := newStore(t.TempDir(), "repo-orphans")
	if err := s.ensureDirs(); err != nil {
		t.Fatalf("ensureDirs() error = %v", err)
	}

	for _, name := range []string{"b.json", "a.json", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(s.orphansPath(), name), []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(s.orphansPath(), "nested"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	got, err := s.orphanManifestPaths()
	if err != nil {
		t.Fatalf("orphanManifestPaths() error = %v", err)
	}

	want := []string{
		filepath.Join(s.orphansPath(), "a.json"),
		filepath.Join(s.orphansPath(), "b.json"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orphanManifestPaths() = %#v, want %#v", got, want)
	}
}

func TestStoreDeleteOrphanManifest(t *testing.T) {
	s := newStore(t.TempDir(), "repo-delete")
	if err := s.ensureDirs(); err != nil {
		t.Fatalf("ensureDirs() error = %v", err)
	}

	id := "orphan-1"
	path := s.orphanManifestPath(id)
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := s.deleteOrphanManifest(id); err != nil {
		t.Fatalf("deleteOrphanManifest() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat() error = %v, want not exist", err)
	}

	if err := s.deleteOrphanManifest(id); err != nil {
		t.Fatalf("deleteOrphanManifest() missing error = %v", err)
	}
}

func TestOrphanIDFromPath(t *testing.T) {
	if got := orphanIDFromPath(filepath.Join("orphans", "abc-123.json")); got != "abc-123" {
		t.Fatalf("orphanIDFromPath() = %q, want %q", got, "abc-123")
	}
}
