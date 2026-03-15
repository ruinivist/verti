package testutil

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func NewRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	RunGit(t, dir, "init")
	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), nil, 0o644); err != nil {
		t.Fatalf("clear exclude: %v", err)
	}
	return dir
}

func NewGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	RunGit(t, dir, "init")
	RunGit(t, dir, "config", "user.name", "Verti Test")
	RunGit(t, dir, "config", "user.email", "verti-test@example.com")

	WriteFile(t, filepath.Join(dir, "README.md"), "# test repo\n")
	RunGit(t, dir, "add", "README.md")
	RunGit(t, dir, "commit", "-m", "chore: initial files")

	return dir
}

func WithWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore dir: %v", err)
		}
	}()

	fn()
}

func NewFakeEditor(t *testing.T, dir, content string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fake editor dir: %v", err)
	}
	path := filepath.Join(dir, "fake-editor")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake editor: %v", err)
	}
	return path
}

func GitRevParse(t *testing.T, dir, rev string) string {
	t.Helper()
	return RunGit(t, dir, "rev-parse", rev)
}

func GitCommonDir(t *testing.T, dir string) string {
	t.Helper()

	commonDir := RunGit(t, dir, "rev-parse", "--git-common-dir")
	if filepath.IsAbs(commonDir) {
		return filepath.Clean(commonDir)
	}
	return filepath.Clean(filepath.Join(dir, commonDir))
}

func VertiConfigPath(t *testing.T, dir string) string {
	t.Helper()
	return filepath.Join(GitCommonDir(t, dir), "verti.toml")
}

func ReferenceTransactionHookPath(t *testing.T, dir string) string {
	t.Helper()
	return filepath.Join(GitCommonDir(t, dir), "hooks", "reference-transaction")
}

func PostCheckoutHookPath(t *testing.T, dir string) string {
	t.Helper()
	return filepath.Join(GitCommonDir(t, dir), "hooks", "post-checkout")
}

func ExcludePath(t *testing.T, dir string) string {
	t.Helper()
	return filepath.Join(GitCommonDir(t, dir), "info", "exclude")
}

func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return string(bytes.TrimSpace(out))
}

func WriteFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
