package snapshots

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSnapshotMissingReturnsFalse(t *testing.T) {
	scopeDir := t.TempDir()

	path, found, err := FindSnapshot(scopeDir, "deadbeef")
	if err != nil {
		t.Fatalf("FindSnapshot() error = %v", err)
	}
	if found {
		t.Fatalf("FindSnapshot() found = true, want false")
	}
	want := filepath.Join(scopeDir, "snapshots", "deadbeef")
	if path != want {
		t.Fatalf("FindSnapshot() path = %q, want %q", path, want)
	}
}

func TestFindSnapshotExistingReturnsPathAndTrue(t *testing.T) {
	scopeDir := t.TempDir()
	want := filepath.Join(scopeDir, "snapshots", "abc123")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatalf("mkdir snapshot dir: %v", err)
	}

	path, found, err := FindSnapshot(scopeDir, "abc123")
	if err != nil {
		t.Fatalf("FindSnapshot() error = %v", err)
	}
	if !found {
		t.Fatalf("FindSnapshot() found = false, want true")
	}
	if path != want {
		t.Fatalf("FindSnapshot() path = %q, want %q", path, want)
	}
}
