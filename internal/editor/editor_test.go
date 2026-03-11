package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEditor(t *testing.T) {
	t.Run("git editor wins", func(t *testing.T) {
		t.Setenv("GIT_EDITOR", "/tmp/git-editor")
		t.Setenv("EDITOR", "/tmp/editor")

		got, err := resolveEditor()
		if err != nil {
			t.Fatalf("resolveEditor() error = %v", err)
		}
		if got != "/tmp/git-editor" {
			t.Fatalf("resolveEditor() = %q, want %q", got, "/tmp/git-editor")
		}
	})

	t.Run("editor fallback", func(t *testing.T) {
		t.Setenv("GIT_EDITOR", "")
		t.Setenv("EDITOR", "/tmp/editor")

		got, err := resolveEditor()
		if err != nil {
			t.Fatalf("resolveEditor() error = %v", err)
		}
		if got != "/tmp/editor" {
			t.Fatalf("resolveEditor() = %q, want %q", got, "/tmp/editor")
		}
	})

	t.Run("micro before vi", func(t *testing.T) {
		binDir := t.TempDir()
		t.Setenv("GIT_EDITOR", "")
		t.Setenv("EDITOR", "")
		t.Setenv("PATH", binDir)

		createEditorBinary(t, filepath.Join(binDir, "micro"), "#!/bin/sh\nexit 0\n")
		createEditorBinary(t, filepath.Join(binDir, "vi"), "#!/bin/sh\nexit 0\n")

		got, err := resolveEditor()
		if err != nil {
			t.Fatalf("resolveEditor() error = %v", err)
		}
		if got != filepath.Join(binDir, "micro") {
			t.Fatalf("resolveEditor() = %q, want %q", got, filepath.Join(binDir, "micro"))
		}
	})

	t.Run("vi fallback", func(t *testing.T) {
		binDir := t.TempDir()
		t.Setenv("GIT_EDITOR", "")
		t.Setenv("EDITOR", "")
		t.Setenv("PATH", binDir)

		createEditorBinary(t, filepath.Join(binDir, "vi"), "#!/bin/sh\nexit 0\n")

		got, err := resolveEditor()
		if err != nil {
			t.Fatalf("resolveEditor() error = %v", err)
		}
		if got != filepath.Join(binDir, "vi") {
			t.Fatalf("resolveEditor() = %q, want %q", got, filepath.Join(binDir, "vi"))
		}
	})

	t.Run("missing editor", func(t *testing.T) {
		t.Setenv("GIT_EDITOR", "")
		t.Setenv("EDITOR", "")
		t.Setenv("PATH", t.TempDir())

		if _, err := resolveEditor(); err == nil {
			t.Fatal("resolveEditor() error = nil, want error")
		}
	})
}

func TestOpenEditorTreatsEnvAsCommandOnly(t *testing.T) {
	binDir := t.TempDir()
	config := filepath.Join(t.TempDir(), "verti.toml")
	t.Setenv("PATH", binDir)
	t.Setenv("GIT_EDITOR", "fake-editor --wait")
	t.Setenv("EDITOR", "")

	createEditorBinary(t, filepath.Join(binDir, "fake-editor"), "#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(config, []byte("test"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := Open(config); err == nil {
		t.Fatal("Open() error = nil, want error")
	}
}

func createEditorBinary(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write editor: %v", err)
	}
}
