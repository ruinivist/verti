package snapshot

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	verticonfig "verti/internal/config"
	"verti/internal/output"
	"verti/internal/testutil"
)

func TestSyncSnapshotsAndRestoresConfiguredArtifacts(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	artifactPath := filepath.Join(repoDir, "test.md")
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := verticonfig.Config{
		RepoID:    "repo-sync",
		Artifacts: []string{"test.md"},
	}
	testutil.WriteFile(t, artifactPath, "snapshot body\n")

	head := testutil.GitRevParse(t, repoDir, "HEAD")

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr, err := runSyncAndCapture(t, cfg)
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
		if stdout != prefixed("Artifacts at snapshot "+head+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("Artifacts at snapshot "+head+"\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}

		testutil.WriteFile(t, artifactPath, "edited body\n")

		stdout, stderr, err = runSyncAndCapture(t, cfg)
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
		if stdout != prefixed("restore "+head+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("restore "+head+"\n"))
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
	snapshotData, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(snapshotData) != "snapshot body\n" {
		t.Fatalf("snapshot = %q, want %q", string(snapshotData), "snapshot body\n")
	}
}

func TestSyncSnapshotsNewHeadWithNestedArtifact(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	artifactPath := filepath.Join(repoDir, "out", "report.txt")
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := verticonfig.Config{
		RepoID:    "repo-nested",
		Artifacts: []string{"out/report.txt"},
	}
	testutil.WriteFile(t, artifactPath, "version one\n")

	testutil.WithWorkingDir(t, repoDir, func() {
		if _, _, err := runSyncAndCapture(t, cfg); err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
	})

	testutil.WriteFile(t, filepath.Join(repoDir, "README.md"), "# updated\n")
	testutil.RunGit(t, repoDir, "add", "README.md")
	testutil.RunGit(t, repoDir, "commit", "-m", "feat: move head")
	testutil.WriteFile(t, artifactPath, "version two\n")

	head := testutil.GitRevParse(t, repoDir, "HEAD")

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr, err := runSyncAndCapture(t, cfg)
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
		if stdout != prefixed("Artifacts at snapshot "+head+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("Artifacts at snapshot "+head+"\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	snapshotPath := filepath.Join(home, storageSubdir, "repo-nested", head, "out", "report.txt")
	snapshotData, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(snapshotData) != "version two\n" {
		t.Fatalf("snapshot = %q, want %q", string(snapshotData), "version two\n")
	}
}

func TestSyncMissingArtifactFails(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	cfg := verticonfig.Config{
		RepoID:    "repo-missing",
		Artifacts: []string{"missing.txt"},
	}

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr, err := runSyncAndCapture(t, cfg)
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
	repoDir := testutil.NewGitRepo(t)
	artifactPath := filepath.Join(repoDir, "test.md")
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := verticonfig.Config{
		RepoID:    "repo-restore",
		Artifacts: []string{"test.md"},
	}
	testutil.WriteFile(t, artifactPath, "snapshot body\n")

	head := testutil.GitRevParse(t, repoDir, "HEAD")
	testutil.WithWorkingDir(t, repoDir, func() {
		if err := Sync(cfg); err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
	})

	snapshotPath := filepath.Join(home, storageSubdir, "repo-restore", head, "test.md")
	if err := os.Remove(snapshotPath); err != nil {
		t.Fatalf("Remove(%q) error = %v", snapshotPath, err)
	}
	testutil.WriteFile(t, artifactPath, "edited body\n")

	testutil.WithWorkingDir(t, repoDir, func() {
		_, _, err := runSyncAndCapture(t, cfg)
		if err == nil {
			t.Fatal("Sync() error = nil, want error")
		}
		if err.Error() != "snapshot missing for artifact: test.md" {
			t.Fatalf("Sync() error = %q, want %q", err.Error(), "snapshot missing for artifact: test.md")
		}
	})
}

func TestSyncNoArtifactsWarns(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	cfg := verticonfig.Config{
		RepoID:    "repo-empty",
		Artifacts: []string{},
	}

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr, err := runSyncAndCapture(t, cfg)
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
		if stdout != prefixed("no artifacts configured\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("no artifacts configured\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})
}

func TestSyncRejectsEscapingArtifactPaths(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	cfg := verticonfig.Config{
		RepoID:    "repo-invalid",
		Artifacts: []string{"../outside.txt"},
	}

	testutil.WithWorkingDir(t, repoDir, func() {
		_, _, err := runSyncAndCapture(t, cfg)
		if err == nil {
			t.Fatal("Sync() error = nil, want error")
		}
		if err.Error() != "invalid artifact path \"../outside.txt\": must not escape repository" {
			t.Fatalf("Sync() error = %q, want %q", err.Error(), "invalid artifact path \"../outside.txt\": must not escape repository")
		}
	})
}

func runSyncAndCapture(t *testing.T, cfg verticonfig.Config) (string, string, error) {
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

	err = Sync(cfg)

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

func prefixed(msg string) string {
	return output.Prefix() + msg
}
