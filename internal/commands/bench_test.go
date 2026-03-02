package commands

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"verti/internal/config"
)

func BenchmarkRunSnapshotFixture(b *testing.B) {
	if _, err := exec.LookPath("git"); err != nil {
		b.Skip("git binary not found in PATH")
	}

	repoDir := createBenchmarkSnapshotRepo(b, 200, 256)
	var stderr bytes.Buffer

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stderr.Reset()
		if err := runSnapshot(repoDir, &stderr); err != nil {
			b.Fatalf("runSnapshot() error = %v", err)
		}
	}
}

func BenchmarkPromptRestoreConfirmation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tty := newFakeTTY("y\n")
		confirmed, err := promptRestoreConfirmation(tty, "benchmark-sha", "main")
		if err != nil {
			b.Fatalf("promptRestoreConfirmation() error = %v", err)
		}
		if !confirmed {
			b.Fatalf("promptRestoreConfirmation() confirmed = false, want true")
		}
	}
}

func createBenchmarkSnapshotRepo(b *testing.B, fileCount int, fileSize int) string {
	b.Helper()

	repoDir := b.TempDir()
	runGitBench(b, repoDir, "init")
	runGitBench(b, repoDir, "config", "user.email", "bench@example.com")
	runGitBench(b, repoDir, "config", "user.name", "Bench User")

	artifactDir := filepath.Join(repoDir, "bench")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		b.Fatalf("mkdir artifact dir: %v", err)
	}

	content := bytes.Repeat([]byte("a"), fileSize)
	for i := 0; i < fileCount; i++ {
		path := filepath.Join(artifactDir, fmt.Sprintf("file-%04d.txt", i))
		if err := os.WriteFile(path, content, 0o644); err != nil {
			b.Fatalf("write benchmark artifact %q: %v", path, err)
		}
	}

	if err := os.WriteFile(filepath.Join(repoDir, "progress.md"), []byte("progress\n"), 0o644); err != nil {
		b.Fatalf("write progress.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		b.Fatalf("write tracked.txt: %v", err)
	}

	runGitBench(b, repoDir, "add", "tracked.txt")
	runGitBench(b, repoDir, "commit", "-m", "benchmark baseline")

	storeRoot := filepath.Join(b.TempDir(), "store")
	cfg := config.Config{
		RepoID:        "bench-repo",
		Enabled:       true,
		Artifacts:     []string{"bench", "progress.md"},
		StoreRoot:     storeRoot,
		RestoreMode:   config.RestoreModePrompt,
		MaxFileSizeMB: config.DefaultMaxFileSizeMB,
	}
	writeRepoConfigBench(b, repoDir, cfg)

	return repoDir
}

func writeRepoConfigBench(b *testing.B, repoDir string, cfg config.Config) {
	b.Helper()
	path := filepath.Join(repoDir, ".git", "verti.toml")
	if err := config.Save(path, cfg); err != nil {
		b.Fatalf("save config at %q: %v", path, err)
	}
}

func runGitBench(b *testing.B, dir string, args ...string) string {
	b.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		b.Fatalf("git %s failed: %v stderr=%q", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String())
}
