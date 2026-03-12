package snapshot

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	verticonfig "verti/internal/config"
	"verti/internal/output"
	"verti/internal/testutil"
)

type syncFixture struct {
	t       *testing.T
	repoDir string
	cfg     verticonfig.Config
	store   store
}

func TestSyncSnapshotsAndRestoresConfiguredArtifacts(t *testing.T) {
	fixture := newSyncFixture(t, "repo-sync", "test.md")
	fixture.writeArtifact("test.md", "snapshot body\n")

	head := fixture.head()
	headDisplay := fixture.headDisplay()

	stdout, stderr, err := fixture.runSync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if stdout != prefixed("Created artifacts for "+headDisplay+"\n") {
		t.Fatalf("stdout = %q, want %q", stdout, prefixed("Created artifacts for "+headDisplay+"\n"))
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	fixture.writeArtifact("test.md", "edited body\n")

	stdout, stderr, err = fixture.runSync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if stdout != prefixed("Restored artifacts at "+headDisplay+"\n") {
		t.Fatalf("stdout = %q, want %q", stdout, prefixed("Restored artifacts at "+headDisplay+"\n"))
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	if got := fixture.readArtifact("test.md"); got != "snapshot body\n" {
		t.Fatalf("artifact = %q, want %q", got, "snapshot body\n")
	}

	storedManifest := fixture.readManifest(head)
	wantHash := hashContent([]byte("snapshot body\n"))
	if storedManifest.Version != manifestVersion {
		t.Fatalf("manifest version = %d, want %d", storedManifest.Version, manifestVersion)
	}
	if got := storedManifest.Artifacts["test.md"]; got != wantHash {
		t.Fatalf("manifest hash = %q, want %q", got, wantHash)
	}
	if got := fixture.readBlob(wantHash); got != "snapshot body\n" {
		t.Fatalf("blob = %q, want %q", got, "snapshot body\n")
	}
}

func TestSyncSnapshotsNewHeadWithNestedArtifact(t *testing.T) {
	fixture := newSyncFixture(t, "repo-nested", "out/report.txt")
	fixture.writeArtifact("out/report.txt", "version one\n")

	if _, _, err := fixture.runSync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	firstHead := fixture.head()

	fixture.advanceHead("feat: move head")
	fixture.writeArtifact("out/report.txt", "version two\n")

	head := fixture.head()
	headDisplay := fixture.headDisplay()

	stdout, stderr, err := fixture.runSync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if stdout != prefixed("Created artifacts for "+headDisplay+"\n") {
		t.Fatalf("stdout = %q, want %q", stdout, prefixed("Created artifacts for "+headDisplay+"\n"))
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	firstManifest := fixture.readManifest(firstHead)
	secondManifest := fixture.readManifest(head)
	firstHash := firstManifest.Artifacts["out/report.txt"]
	secondHash := secondManifest.Artifacts["out/report.txt"]
	if firstHash == secondHash {
		t.Fatalf("hashes = %q and %q, want different values", firstHash, secondHash)
	}
	if got := fixture.readBlob(secondHash); got != "version two\n" {
		t.Fatalf("blob = %q, want %q", got, "version two\n")
	}
}

func TestSyncReusesBlobAcrossCommitsWithIdenticalContent(t *testing.T) {
	fixture := newSyncFixture(t, "repo-dedup", "test.md")
	fixture.writeArtifact("test.md", "same content\n")

	if _, _, err := fixture.runSync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	firstHead := fixture.head()

	fixture.advanceHead("feat: move head")
	fixture.writeArtifact("test.md", "same content\n")

	if _, _, err := fixture.runSync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	secondHead := fixture.head()

	firstManifest := fixture.readManifest(firstHead)
	secondManifest := fixture.readManifest(secondHead)
	firstHash := firstManifest.Artifacts["test.md"]
	secondHash := secondManifest.Artifacts["test.md"]
	if firstHash != secondHash {
		t.Fatalf("hashes = %q and %q, want same value", firstHash, secondHash)
	}

	entries, err := os.ReadDir(fixture.store.blobsPath())
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("blob count = %d, want 1", len(entries))
	}
}

func TestSyncMissingArtifactFails(t *testing.T) {
	fixture := newSyncFixture(t, "repo-missing", "missing.txt")

	stdout, stderr, err := fixture.runSync()
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

	if _, err := os.Stat(fixture.store.manifestPath(fixture.head())); !os.IsNotExist(err) {
		t.Fatalf("manifest stat error = %v, want not exist", err)
	}
}

func TestSyncMissingManifestArtifactFails(t *testing.T) {
	fixture := newSyncFixture(t, "repo-restore", "test.md")
	fixture.writeArtifact("test.md", "snapshot body\n")

	head := fixture.head()
	fixture.mustSync()
	fixture.writeManifest(head, manifest{
		Version:   manifestVersion,
		Artifacts: map[string]string{},
	})
	fixture.writeArtifact("test.md", "edited body\n")

	_, _, err := fixture.runSync()
	if err == nil {
		t.Fatal("Sync() error = nil, want error")
	}
	if err.Error() != "manifest missing artifact: test.md" {
		t.Fatalf("Sync() error = %q, want %q", err.Error(), "manifest missing artifact: test.md")
	}
}

func TestSyncInvalidManifestHashFails(t *testing.T) {
	fixture := newSyncFixture(t, "repo-invalid-hash", "test.md")
	fixture.writeArtifact("test.md", "snapshot body\n")

	head := fixture.head()
	fixture.mustSync()
	fixture.writeManifest(head, manifest{
		Version: manifestVersion,
		Artifacts: map[string]string{
			"test.md": "",
		},
	})
	fixture.writeArtifact("test.md", "edited body\n")

	_, _, err := fixture.runSync()
	if err == nil {
		t.Fatal("Sync() error = nil, want error")
	}
	if err.Error() != "invalid manifest hash for artifact: test.md" {
		t.Fatalf("Sync() error = %q, want %q", err.Error(), "invalid manifest hash for artifact: test.md")
	}
}

func TestSyncMissingBlobFails(t *testing.T) {
	fixture := newSyncFixture(t, "repo-missing-blob", "test.md")
	fixture.writeArtifact("test.md", "snapshot body\n")

	head := fixture.head()
	fixture.mustSync()

	storedManifest := fixture.readManifest(head)
	if err := os.Remove(fixture.store.blobPath(storedManifest.Artifacts["test.md"])); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	fixture.writeArtifact("test.md", "edited body\n")

	_, _, err := fixture.runSync()
	if err == nil {
		t.Fatal("Sync() error = nil, want error")
	}
	if err.Error() != "blob missing for artifact: test.md" {
		t.Fatalf("Sync() error = %q, want %q", err.Error(), "blob missing for artifact: test.md")
	}
}

func TestSyncNoArtifactsWarns(t *testing.T) {
	fixture := newSyncFixture(t, "repo-empty")

	stdout, stderr, err := fixture.runSync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if stdout != prefixed("no artifacts configured\n") {
		t.Fatalf("stdout = %q, want %q", stdout, prefixed("no artifacts configured\n"))
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestSyncRejectsEscapingArtifactPaths(t *testing.T) {
	fixture := newSyncFixture(t, "repo-invalid", "../outside.txt")

	_, _, err := fixture.runSync()
	if err == nil {
		t.Fatal("Sync() error = nil, want error")
	}
	if err.Error() != "invalid artifact path \"../outside.txt\": must not escape repository" {
		t.Fatalf("Sync() error = %q, want %q", err.Error(), "invalid artifact path \"../outside.txt\": must not escape repository")
	}
}

func newSyncFixture(t *testing.T, repoID string, artifacts ...string) syncFixture {
	t.Helper()

	repoDir := testutil.NewGitRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	return syncFixture{
		t:       t,
		repoDir: repoDir,
		cfg: verticonfig.Config{
			RepoID:    repoID,
			Artifacts: artifacts,
		},
		store: newStore(home, repoID),
	}
}

func (f syncFixture) artifactPath(path string) string {
	return filepath.Join(f.repoDir, path)
}

func (f syncFixture) writeArtifact(path, content string) {
	f.t.Helper()
	testutil.WriteFile(f.t, f.artifactPath(path), content)
}

func (f syncFixture) readArtifact(path string) string {
	f.t.Helper()

	content, err := os.ReadFile(f.artifactPath(path))
	if err != nil {
		f.t.Fatalf("read artifact: %v", err)
	}
	return string(content)
}

func (f syncFixture) readBlob(hash string) string {
	f.t.Helper()

	content, err := os.ReadFile(f.store.blobPath(hash))
	if err != nil {
		f.t.Fatalf("read blob: %v", err)
	}
	return string(content)
}

func (f syncFixture) readManifest(commit string) manifest {
	f.t.Helper()
	return readManifest(f.t, f.store.manifestPath(commit))
}

func (f syncFixture) writeManifest(commit string, storedManifest manifest) {
	f.t.Helper()
	writeManifestFile(f.t, f.store.manifestPath(commit), storedManifest)
}

func (f syncFixture) runSync() (string, string, error) {
	f.t.Helper()

	var stdout, stderr string
	var err error
	testutil.WithWorkingDir(f.t, f.repoDir, func() {
		stdout, stderr, err = runSyncAndCapture(f.t, f.cfg)
	})
	return stdout, stderr, err
}

func (f syncFixture) mustSync() {
	f.t.Helper()

	if _, _, err := f.runSync(); err != nil {
		f.t.Fatalf("Sync() error = %v", err)
	}
}

func (f syncFixture) head() string {
	f.t.Helper()
	return testutil.GitRevParse(f.t, f.repoDir, "HEAD")
}

func (f syncFixture) headDisplay() string {
	f.t.Helper()
	return testutil.RunGit(f.t, f.repoDir, "show", "-s", "--format=%s [%h]", "HEAD")
}

func (f syncFixture) advanceHead(message string) {
	f.t.Helper()
	testutil.WriteFile(f.t, filepath.Join(f.repoDir, "README.md"), "# updated\n")
	testutil.RunGit(f.t, f.repoDir, "add", "README.md")
	testutil.RunGit(f.t, f.repoDir, "commit", "-m", message)
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

func readManifest(t *testing.T, path string) manifest {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var storedManifest manifest
	if err := json.Unmarshal(content, &storedManifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	return storedManifest
}

func writeManifestFile(t *testing.T, path string, storedManifest manifest) {
	t.Helper()

	content, err := json.MarshalIndent(storedManifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func prefixed(msg string) string {
	return output.Format(msg)
}
