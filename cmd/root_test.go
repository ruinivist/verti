package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
			wantOut:  prefixed("Done initialising...\n"),
			wantCode: 0,
		},
		{
			name:     "add",
			args:     []string{"add", "notes.txt"},
			wantOut:  prefixed("Added artifact: notes.txt\n"),
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
		{
			name:     "add missing path",
			args:     []string{"add"},
			wantOut:  prefixed("usage: verti add <path>\n"),
			wantCode: 1,
		},
		{
			name:     "add extra",
			args:     []string{"add", "notes.txt", "extra"},
			wantOut:  prefixed("unknown add option: extra\n"),
			wantCode: 1,
		},
		{
			name:     "orphans extra",
			args:     []string{"orphans", "1", "extra"},
			wantOut:  prefixed("unknown orphans option: extra\n"),
			wantCode: 1,
		},
		{
			name:     "orphans invalid",
			args:     []string{"orphans", "wat"},
			wantOut:  prefixed("invalid orphan number: wat\n"),
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

			if tt.name == "init" || tt.name == "add" {
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

		if stdout != prefixed("Done initialising...\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("Done initialising...\n"))
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

func TestRunAddExecution(t *testing.T) {
	repoDir := testutil.NewRepo(t)
	t.Setenv("GIT_EDITOR", testutil.NewFakeEditor(t, repoDir, "#!/bin/sh\nexit 99\n"))
	t.Setenv("EDITOR", "")

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"add", "docs/../notes.txt"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		if stdout != prefixed("Added artifact: notes.txt\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("Added artifact: notes.txt\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}

		config, err := os.ReadFile(filepath.Join(repoDir, ".git", "verti.toml"))
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		if !bytes.Contains(config, []byte("repo_id = ")) || !bytes.Contains(config, []byte("artifacts = [\"notes.txt\"]\n")) {
			t.Fatalf("config missing added artifact: %q", string(config))
		}

		exclude, err := os.ReadFile(filepath.Join(repoDir, ".git", "info", "exclude"))
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		if string(exclude) != "notes.txt\n" {
			t.Fatalf("exclude = %q, want %q", string(exclude), "notes.txt\n")
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
	headDisplay := testutil.RunGit(t, repoDir, "show", "-s", "--format=%s [%h]", "HEAD")
	wantHash := sha256Hex([]byte("snapshot body\n"))

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"sync"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		if stdout != prefixed("Created artifacts for "+headDisplay+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("Created artifacts for "+headDisplay+"\n"))
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

		if stdout != prefixed("Restored artifacts at "+headDisplay+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("Restored artifacts at "+headDisplay+"\n"))
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

	repoStoreDir := filepath.Join(home, ".verti", "repos", "repo-cmd-sync")
	manifestPath := filepath.Join(repoStoreDir, "manifests", head+".json")
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest struct {
		Version   int               `json:"version"`
		CreatedAt string            `json:"created_at"`
		Artifacts map[string]string `json:"artifacts"`
	}
	if err := json.Unmarshal(manifestContent, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest.Version != 1 {
		t.Fatalf("manifest version = %d, want 1", manifest.Version)
	}
	if manifest.CreatedAt == "" {
		t.Fatal("manifest created_at is empty")
	}
	if _, err := time.Parse(time.RFC3339, manifest.CreatedAt); err != nil {
		t.Fatalf("manifest created_at parse error = %v", err)
	}
	if got := manifest.Artifacts["test.md"]; got != wantHash {
		t.Fatalf("manifest hash = %q, want %q", got, wantHash)
	}

	blob, err := os.ReadFile(filepath.Join(repoStoreDir, "blobs", wantHash))
	if err != nil {
		t.Fatalf("read blob: %v", err)
	}
	if string(blob) != "snapshot body\n" {
		t.Fatalf("blob = %q, want %q", string(blob), "snapshot body\n")
	}
}

func TestRunSyncSnapshotAndRestoreDirectoryArtifact(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := verticonfig.WriteConfig(filepath.Join(repoDir, ".git", "verti.toml"), verticonfig.Config{
		RepoID:    "repo-cmd-dir",
		Artifacts: []string{"docs"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}
	testutil.WriteFile(t, filepath.Join(repoDir, "docs", "guide.md"), "guide body\n")
	testutil.WriteFile(t, filepath.Join(repoDir, "docs", "nested", "notes.txt"), "notes body\n")

	head := testutil.GitRevParse(t, repoDir, "HEAD")
	headDisplay := testutil.RunGit(t, repoDir, "show", "-s", "--format=%s [%h]", "HEAD")

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"sync"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		if stdout != prefixed("Created artifacts for "+headDisplay+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("Created artifacts for "+headDisplay+"\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}

		testutil.WriteFile(t, filepath.Join(repoDir, "docs", "guide.md"), "edited body\n")
		testutil.WriteFile(t, filepath.Join(repoDir, "docs", "extra.txt"), "keep me\n")
		if err := os.Remove(filepath.Join(repoDir, "docs", "nested", "notes.txt")); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}

		stdout, stderr = captureOutput(t, func() {
			if got := Run([]string{"sync"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		if stdout != prefixed("Restored artifacts at "+headDisplay+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("Restored artifacts at "+headDisplay+"\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	guide, err := os.ReadFile(filepath.Join(repoDir, "docs", "guide.md"))
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	if string(guide) != "guide body\n" {
		t.Fatalf("guide = %q, want %q", string(guide), "guide body\n")
	}

	notes, err := os.ReadFile(filepath.Join(repoDir, "docs", "nested", "notes.txt"))
	if err != nil {
		t.Fatalf("read notes: %v", err)
	}
	if string(notes) != "notes body\n" {
		t.Fatalf("notes = %q, want %q", string(notes), "notes body\n")
	}

	extra, err := os.ReadFile(filepath.Join(repoDir, "docs", "extra.txt"))
	if err != nil {
		t.Fatalf("read extra: %v", err)
	}
	if string(extra) != "keep me\n" {
		t.Fatalf("extra = %q, want %q", string(extra), "keep me\n")
	}

	repoStoreDir := filepath.Join(home, ".verti", "repos", "repo-cmd-dir")
	manifestPath := filepath.Join(repoStoreDir, "manifests", head+".json")
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest struct {
		Version   int               `json:"version"`
		CreatedAt string            `json:"created_at"`
		Artifacts map[string]string `json:"artifacts"`
	}
	if err := json.Unmarshal(manifestContent, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest.Version != 1 {
		t.Fatalf("manifest version = %d, want 1", manifest.Version)
	}
	if manifest.CreatedAt == "" {
		t.Fatal("manifest created_at is empty")
	}
	if _, err := time.Parse(time.RFC3339, manifest.CreatedAt); err != nil {
		t.Fatalf("manifest created_at parse error = %v", err)
	}
	if got := manifest.Artifacts["docs/guide.md"]; got != sha256Hex([]byte("guide body\n")) {
		t.Fatalf("guide hash = %q, want %q", got, sha256Hex([]byte("guide body\n")))
	}
	if got := manifest.Artifacts["docs/nested/notes.txt"]; got != sha256Hex([]byte("notes body\n")) {
		t.Fatalf("notes hash = %q, want %q", got, sha256Hex([]byte("notes body\n")))
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

func TestRunOrphansEmpty(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := verticonfig.WriteConfig(filepath.Join(repoDir, ".git", "verti.toml"), verticonfig.Config{
		RepoID:    "repo-cmd-orphans-empty",
		Artifacts: []string{"test.md"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"orphans"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		if stdout != prefixed("no orphan snapshots\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("no orphan snapshots\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})
}

func TestRunOrphansListsNewestFirst(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	oldLocal := time.Local
	time.Local = time.FixedZone("UTC", 0)
	t.Cleanup(func() {
		time.Local = oldLocal
	})

	repoID := "repo-cmd-orphans-list"
	if err := verticonfig.WriteConfig(filepath.Join(repoDir, ".git", "verti.toml"), verticonfig.Config{
		RepoID:    repoID,
		Artifacts: []string{"test.md"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	now := time.Now()
	recent := now.Add(-2 * time.Hour)
	older := now.Add(-26 * time.Hour)
	writeCmdOrphanManifest(t, home, repoID, "orphan-recent", recent, map[string]string{
		"test.md": writeCmdBlob(t, home, repoID, "recent body\n"),
	})
	writeCmdOrphanManifest(t, home, repoID, "orphan-old", older, map[string]string{
		"test.md":  writeCmdBlob(t, home, repoID, "old body\n"),
		"docs.txt": writeCmdBlob(t, home, repoID, "other\n"),
	})

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"orphans"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		first := prefixed(fmt.Sprintf("1. 2 hours ago (%s) - 1 artifact\n", recent.In(time.Local).Format("2006-01-02 15:04:05 -0700")))
		second := prefixed(fmt.Sprintf("2. 1 day ago (%s) - 2 artifacts\n", older.In(time.Local).Format("2006-01-02 15:04:05 -0700")))
		if !strings.Contains(stdout, first) {
			t.Fatalf("stdout = %q, want first list entry %q", stdout, first)
		}
		if !strings.Contains(stdout, second) {
			t.Fatalf("stdout = %q, want second list entry %q", stdout, second)
		}
		if strings.Index(stdout, first) > strings.Index(stdout, second) {
			t.Fatalf("stdout order = %q, want newest first", stdout)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})
}

func TestRunOrphansRestore(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoID := "repo-cmd-orphan-restore"
	if err := verticonfig.WriteConfig(filepath.Join(repoDir, ".git", "verti.toml"), verticonfig.Config{
		RepoID:    repoID,
		Artifacts: []string{"test.md"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}
	testutil.WriteFile(t, filepath.Join(repoDir, "test.md"), "current local\n")

	writeCmdOrphanManifest(t, home, repoID, "orphan-recent", time.Now().Add(-time.Hour), map[string]string{
		"test.md": writeCmdBlob(t, home, repoID, "recent orphan\n"),
	})
	writeCmdOrphanManifest(t, home, repoID, "orphan-old", time.Now().Add(-2*time.Hour), map[string]string{
		"test.md": writeCmdBlob(t, home, repoID, "old orphan\n"),
	})

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"orphans", "2"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		if stdout != prefixed("Restored orphan #2 (orphan-old)\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("Restored orphan #2 (orphan-old)\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	content, err := os.ReadFile(filepath.Join(repoDir, "test.md"))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(content) != "old orphan\n" {
		t.Fatalf("artifact = %q, want %q", string(content), "old orphan\n")
	}
}

func TestRunOrphansOutOfRange(t *testing.T) {
	repoDir := testutil.NewGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoID := "repo-cmd-orphan-range"
	if err := verticonfig.WriteConfig(filepath.Join(repoDir, ".git", "verti.toml"), verticonfig.Config{
		RepoID:    repoID,
		Artifacts: []string{"test.md"},
	}); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}
	writeCmdOrphanManifest(t, home, repoID, "orphan-only", time.Now().Add(-time.Hour), map[string]string{
		"test.md": writeCmdBlob(t, home, repoID, "body\n"),
	})

	testutil.WithWorkingDir(t, repoDir, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"orphans", "2"}); got != 1 {
				t.Fatalf("Run() code = %d, want %d", got, 1)
			}
		})

		if stdout != prefixed("orphan number out of range: 2\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed("orphan number out of range: 2\n"))
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
	return output.Format(msg)
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func writeCmdBlob(t *testing.T, home, repoID, content string) string {
	t.Helper()

	hash := sha256Hex([]byte(content))
	path := filepath.Join(home, ".verti", "repos", repoID, "blobs", hash)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return hash
}

func writeCmdOrphanManifest(t *testing.T, home, repoID, id string, createdAt time.Time, artifacts map[string]string) {
	t.Helper()

	path := filepath.Join(home, ".verti", "repos", repoID, "orphans", id+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	content, err := json.MarshalIndent(struct {
		Version   int               `json:"version"`
		CreatedAt string            `json:"created_at"`
		Artifacts map[string]string `json:"artifacts"`
	}{
		Version:   1,
		CreatedAt: createdAt.UTC().Format(time.RFC3339),
		Artifacts: artifacts,
	}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
