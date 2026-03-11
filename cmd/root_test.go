package cmd

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

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantErr  string
		wantCode int
	}{
		{
			name:     "no args",
			wantOut:  prefixed("unknown command\n"),
			wantCode: 1,
		},
		{
			name:     "init",
			args:     []string{"init"},
			wantOut:  prefixed("init\n"),
			wantCode: 0,
		},
		{
			name:     "unknown command",
			args:     []string{"wat"},
			wantOut:  prefixed("unknown command: wat\n"),
			wantCode: 1,
		},
		{
			name:     "sync extra",
			args:     []string{"sync", "extra", "bits"},
			wantOut:  prefixed("unknown sync option: extra bits\n"),
			wantCode: 1,
		},
		{
			name:     "init extra",
			args:     []string{"init", "extra"},
			wantOut:  prefixed("unknown init option: extra\n"),
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := func() {
				stdout, stderr := captureOutput(t, func() {
					if got := Run(tt.args); got != tt.wantCode {
						t.Fatalf("Run() code = %d, want %d", got, tt.wantCode)
					}
				})

				if stdout != tt.wantOut {
					t.Fatalf("stdout = %q, want %q", stdout, tt.wantOut)
				}
				if stderr != tt.wantErr {
					t.Fatalf("stderr = %q, want %q", stderr, tt.wantErr)
				}
			}

			if tt.name == "init" {
				repoDir := testutil.NewRepo(t)
				t.Setenv("GIT_EDITOR", testutil.NewFakeEditor(t, repoDir, "#!/bin/sh\nexit 0\n"))
				t.Setenv("EDITOR", "")
				testutil.WithWorkingDir(t, repoDir, run)
				return
			}

			run()
		})
	}
}

func TestRunInitExecution(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	t.Setenv("GIT_EDITOR", testutil.NewFakeEditor(t, repoDir, "#!/bin/sh\ncat <<'EOF' > \"$1\"\n[verti]\nrepo_id = \"repo-cmd\"\nartifacts = [\"test.md\", \"out/report.txt\"]\nEOF\n"))
	t.Setenv("EDITOR", "")

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"init"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		if stdout != prefixed("init\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("init\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}

		config, err := os.ReadFile(filepath.Join(repoDir, ".git", "verti.toml"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		if !bytes.Contains(config, []byte("repo_id = \"repo-cmd\"\n")) || !bytes.Contains(config, []byte("artifacts = [\"test.md\", \"out/report.txt\"]\n")) {
			t.Fatalf("config missing edited content: %q", string(config))
		}

		exclude, err := os.ReadFile(filepath.Join(repoDir, ".git", "info", "exclude"))
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if string(exclude) != "test.md\nout/report.txt\n" {
			t.Fatalf("exclude = %q, want %q", string(exclude), "test.md\nout/report.txt\n")
		}
	})
}

func TestRunSyncSnapshotAndRestore(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	artifactPath := filepath.Join(repoDir, "test.md")
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := verticonfig.WriteConfig(filepath.Join(repoDir, ".git", "verti.toml"), verticonfig.Config{
		RepoID:    "repo-cmd-sync",
		Artifacts: []string{"test.md"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}
	testutil.WriteFile(t, artifactPath, "snapshot body\n")

	head := testutil.GitRevParse(t, repoDir, "HEAD")

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"sync"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		if stdout != prefixed("snapshot "+head+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("snapshot "+head+"\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}

		testutil.WriteFile(t, artifactPath, "edited body\n")

		stdout, stderr = captureOutput(t, func() {
			if got := Run([]string{"sync"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

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

	snapshotPath := filepath.Join(home, ".verti", "repos", "repo-cmd-sync", head, "test.md")
	snapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(snapshot) != "snapshot body\n" {
		t.Fatalf("snapshot = %q, want %q", string(snapshot), "snapshot body\n")
	}
}

func TestRunSyncNoArtifacts(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	t.Setenv("HOME", t.TempDir())

	if err := verticonfig.WriteConfig(filepath.Join(repoDir, ".git", "verti.toml"), verticonfig.Config{
		RepoID:    "repo-cmd-empty",
		Artifacts: []string{},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"sync"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		if stdout != prefixed("no artifacts configured\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("no artifacts configured\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})
}

func captureOutput(t *testing.T, fn func()) (string, string) {
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

	fn()

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

	return stdoutBuf.String(), stderrBuf.String()
}

func prefixed(msg string) string {
	return output.Prefix() + msg
}
