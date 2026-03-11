package verti

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSyncSnapshotsAndRestoresConfiguredArtifacts(t *testing.T) {
	repoDir := newGitRepo(t)
	artifactPath := filepath.Join(repoDir, "test.md")
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := WriteConfig(filepath.Join(repoDir, configPath), Config{
		RepoID:    "repo-sync",
		Artifacts: []string{"test.md"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}
	writeRepoFile(t, artifactPath, "snapshot body\n")

	head := gitRevParse(t, repoDir, "HEAD")

	withWorkingDir(t, repoDir, func() {
		stdout, stderr, err := runSyncAndCapture(t)
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
		if stdout != "snapshot "+head+"\n" {
			t.Fatalf("stdout = %q, want %q", stdout, "snapshot "+head+"\n")
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}

		writeRepoFile(t, artifactPath, "edited body\n")

		stdout, stderr, err = runSyncAndCapture(t)
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
		if stdout != "restore "+head+"\n" {
			t.Fatalf("stdout = %q, want %q", stdout, "restore "+head+"\n")
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	content, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(content) != "snapshot body\n" {
		t.Fatalf("artifact = %q, want %q", string(content), "snapshot body\n")
	}

	snapshotPath := filepath.Join(home, storageSubdir, "repo-sync", head, "test.md")
	snapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(snapshot) != "snapshot body\n" {
		t.Fatalf("snapshot = %q, want %q", string(snapshot), "snapshot body\n")
	}
}

func TestSyncSnapshotsNewHeadWithNestedArtifact(t *testing.T) {
	repoDir := newGitRepo(t)
	artifactPath := filepath.Join(repoDir, "out", "report.txt")
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := WriteConfig(filepath.Join(repoDir, configPath), Config{
		RepoID:    "repo-nested",
		Artifacts: []string{"out/report.txt"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}
	writeRepoFile(t, artifactPath, "version one\n")

	withWorkingDir(t, repoDir, func() {
		if _, _, err := runSyncAndCapture(t); err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
	})

	writeRepoFile(t, filepath.Join(repoDir, "README.md"), "# updated\n")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "feat: move head")
	writeRepoFile(t, artifactPath, "version two\n")

	head := gitRevParse(t, repoDir, "HEAD")

	withWorkingDir(t, repoDir, func() {
		stdout, stderr, err := runSyncAndCapture(t)
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
		if stdout != "snapshot "+head+"\n" {
			t.Fatalf("stdout = %q, want %q", stdout, "snapshot "+head+"\n")
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	snapshotPath := filepath.Join(home, storageSubdir, "repo-nested", head, "out", "report.txt")
	snapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(snapshot) != "version two\n" {
		t.Fatalf("snapshot = %q, want %q", string(snapshot), "version two\n")
	}
}

func TestSyncMissingArtifactFails(t *testing.T) {
	repoDir := newGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	if err := WriteConfig(filepath.Join(repoDir, configPath), Config{
		RepoID:    "repo-missing",
		Artifacts: []string{"missing.txt"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	withWorkingDir(t, repoDir, func() {
		stdout, stderr, err := runSyncAndCapture(t)
		if err == nil {
			t.Fatal("Sync() error = nil, want error")
		}
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if err.Error() != "artifact not found: missing.txt" {
			t.Fatalf("Sync() error = %q, want %q", err.Error(), "artifact not found: missing.txt")
		}
	})
}

func TestSyncMissingStoredArtifactFails(t *testing.T) {
	repoDir := newGitRepo(t)
	artifactPath := filepath.Join(repoDir, "test.md")
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := WriteConfig(filepath.Join(repoDir, configPath), Config{
		RepoID:    "repo-restore",
		Artifacts: []string{"test.md"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}
	writeRepoFile(t, artifactPath, "snapshot body\n")

	head := gitRevParse(t, repoDir, "HEAD")
	withWorkingDir(t, repoDir, func() {
		if err := Sync(); err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
	})

	snapshotPath := filepath.Join(home, storageSubdir, "repo-restore", head, "test.md")
	if err := os.Remove(snapshotPath); err != nil {
		t.Fatalf("Remove(%q) error = %v", snapshotPath, err)
	}
	writeRepoFile(t, artifactPath, "edited body\n")

	withWorkingDir(t, repoDir, func() {
		_, _, err := runSyncAndCapture(t)
		if err == nil {
			t.Fatal("Sync() error = nil, want error")
		}
		if err.Error() != "snapshot missing for artifact: test.md" {
			t.Fatalf("Sync() error = %q, want %q", err.Error(), "snapshot missing for artifact: test.md")
		}
	})
}

func TestSyncNoArtifactsWarns(t *testing.T) {
	repoDir := newGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	if err := WriteConfig(filepath.Join(repoDir, configPath), Config{
		RepoID:    "repo-empty",
		Artifacts: []string{},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	withWorkingDir(t, repoDir, func() {
		stdout, stderr, err := runSyncAndCapture(t)
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
		if stdout != "no artifacts configured\n" {
			t.Fatalf("stdout = %q, want %q", stdout, "no artifacts configured\n")
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})
}

func TestSyncOutsideGitRepoFails(t *testing.T) {
	withWorkingDir(t, t.TempDir(), func() {
		err := Sync()
		if err == nil {
			t.Fatal("Sync() error = nil, want error")
		}
		if err.Error() != "not a git repository" {
			t.Fatalf("Sync() error = %q, want %q", err.Error(), "not a git repository")
		}
	})
}

func TestSyncRejectsEscapingArtifactPaths(t *testing.T) {
	repoDir := newGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	if err := WriteConfig(filepath.Join(repoDir, configPath), Config{
		RepoID:    "repo-invalid",
		Artifacts: []string{"../outside.txt"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	withWorkingDir(t, repoDir, func() {
		_, _, err := runSyncAndCapture(t)
		if err == nil {
			t.Fatal("Sync() error = nil, want error")
		}
		if err.Error() != "invalid artifact path \"../outside.txt\": must not escape repository" {
			t.Fatalf("Sync() error = %q, want %q", err.Error(), "invalid artifact path \"../outside.txt\": must not escape repository")
		}
	})
}

func runSyncAndCapture(t *testing.T) (string, string, error) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdout: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stderr: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	err = Sync()

	if err := stdoutW.Close(); err != nil {
		t.Fatalf("stdout close: %v", err)
	}
	if err := stderrW.Close(); err != nil {
		t.Fatalf("stderr close: %v", err)
	}

	var stdoutBuf bytes.Buffer
	if _, err := io.Copy(&stdoutBuf, stdoutR); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	var stderrBuf bytes.Buffer
	if _, err := io.Copy(&stderrBuf, stderrR); err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	if err := stdoutR.Close(); err != nil {
		t.Fatalf("stdout reader close: %v", err)
	}
	if err := stderrR.Close(); err != nil {
		t.Fatalf("stderr reader close: %v", err)
	}

	return stdoutBuf.String(), stderrBuf.String(), err
}

func newGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Verti Test")
	runGit(t, dir, "config", "user.email", "verti-test@example.com")

	writeRepoFile(t, filepath.Join(dir, "README.md"), "# test repo\n")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "chore: initial files")

	return dir
}

func gitRevParse(t *testing.T, dir string, rev string) string {
	t.Helper()
	return runGit(t, dir, "rev-parse", rev)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return string(bytes.TrimSpace(out))
}

func writeRepoFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
