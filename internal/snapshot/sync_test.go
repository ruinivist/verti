package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

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
	if storedManifest.CreatedAt == "" {
		t.Fatal("manifest created_at is empty")
	}
	if _, err := time.Parse(time.RFC3339, storedManifest.CreatedAt); err != nil {
		t.Fatalf("manifest created_at parse error = %v", err)
	}
	if got := storedManifest.Artifacts["test.md"]; got != wantHash {
		t.Fatalf("manifest hash = %q, want %q", got, wantHash)
	}
	if got := fixture.readBlob(wantHash); got != "snapshot body\n" {
		t.Fatalf("blob = %q, want %q", got, "snapshot body\n")
	}

	orphanManifests := fixture.readOrphanManifests()
	if len(orphanManifests) != 1 {
		t.Fatalf("orphan manifest count = %d, want 1", len(orphanManifests))
	}
	for id, storedOrphan := range orphanManifests {
		if _, err := uuid.Parse(id); err != nil {
			t.Fatalf("orphan manifest id %q parse error = %v", id, err)
		}
		if storedOrphan.CreatedAt == "" {
			t.Fatal("orphan manifest created_at is empty")
		}
		if _, err := time.Parse(time.RFC3339, storedOrphan.CreatedAt); err != nil {
			t.Fatalf("orphan manifest created_at parse error = %v", err)
		}
		if got := storedOrphan.Artifacts["test.md"]; got != hashContent([]byte("edited body\n")) {
			t.Fatalf("orphan manifest hash = %q, want %q", got, hashContent([]byte("edited body\n")))
		}
		hash, ok := storedOrphan.Artifacts["test.md"]
		if !ok {
			t.Fatal("orphan manifest missing test.md hash")
		}
		if got := fixture.readBlob(hash); got != "edited body\n" {
			t.Fatalf("orphan blob = %q, want %q", got, "edited body\n")
		}
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

func TestSyncSnapshotsAndRestoresConfiguredDirectory(t *testing.T) {
	fixture := newSyncFixture(t, "repo-dir", "docs")
	fixture.writeArtifact("docs/guide.md", "guide v1\n")
	fixture.writeArtifact("docs/nested/notes.txt", "notes v1\n")
	if err := os.MkdirAll(fixture.artifactPath("docs/empty"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

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

	storedManifest := fixture.readManifest(head)
	if len(storedManifest.Artifacts) != 2 {
		t.Fatalf("manifest entries = %d, want 2", len(storedManifest.Artifacts))
	}
	if got := storedManifest.Artifacts["docs/guide.md"]; got != hashContent([]byte("guide v1\n")) {
		t.Fatalf("manifest hash = %q, want %q", got, hashContent([]byte("guide v1\n")))
	}
	if got := storedManifest.Artifacts["docs/nested/notes.txt"]; got != hashContent([]byte("notes v1\n")) {
		t.Fatalf("manifest hash = %q, want %q", got, hashContent([]byte("notes v1\n")))
	}

	fixture.writeArtifact("docs/guide.md", "guide edited\n")
	if err := os.Remove(fixture.artifactPath("docs/nested/notes.txt")); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	fixture.writeArtifact("docs/extra.txt", "leave me alone\n")

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

	if got := fixture.readArtifact("docs/guide.md"); got != "guide v1\n" {
		t.Fatalf("guide = %q, want %q", got, "guide v1\n")
	}
	if got := fixture.readArtifact("docs/nested/notes.txt"); got != "notes v1\n" {
		t.Fatalf("notes = %q, want %q", got, "notes v1\n")
	}
	if got := fixture.readArtifact("docs/extra.txt"); got != "leave me alone\n" {
		t.Fatalf("extra = %q, want %q", got, "leave me alone\n")
	}

	orphanManifests := fixture.readOrphanManifests()
	if len(orphanManifests) != 1 {
		t.Fatalf("orphan manifest count = %d, want 1", len(orphanManifests))
	}
	for _, storedOrphan := range orphanManifests {
		if len(storedOrphan.Artifacts) != 1 {
			t.Fatalf("orphan manifest entries = %d, want 1", len(storedOrphan.Artifacts))
		}
		if got := storedOrphan.Artifacts["docs/guide.md"]; got != hashContent([]byte("guide edited\n")) {
			t.Fatalf("orphan manifest hash = %q, want %q", got, hashContent([]byte("guide edited\n")))
		}
	}
}

func TestSyncRestoresConfiguredDirectoryWhenMissingLocally(t *testing.T) {
	fixture := newSyncFixture(t, "repo-dir-missing", "docs")
	fixture.writeArtifact("docs/guide.md", "guide v1\n")

	headDisplay := fixture.headDisplay()
	fixture.mustSync()

	if err := os.RemoveAll(fixture.artifactPath("docs")); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	stdout, stderr, err := fixture.runSync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if stdout != prefixed("Restored artifacts at "+headDisplay+"\n") {
		t.Fatalf("stdout = %q, want %q", stdout, prefixed("Restored artifacts at "+headDisplay+"\n"))
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if got := fixture.readArtifact("docs/guide.md"); got != "guide v1\n" {
		t.Fatalf("guide = %q, want %q", got, "guide v1\n")
	}
	if got := fixture.orphanManifestCount(); got != 0 {
		t.Fatalf("orphan manifest count = %d, want 0", got)
	}
}

func TestSyncRestoreWithMultipleDifferingFilesCreatesSingleOrphanManifest(t *testing.T) {
	fixture := newSyncFixture(t, "repo-multi-orphan", "docs")
	fixture.writeArtifact("docs/guide.md", "guide v1\n")
	fixture.writeArtifact("docs/notes.txt", "notes v1\n")

	headDisplay := fixture.headDisplay()
	fixture.mustSync()

	fixture.writeArtifact("docs/guide.md", "guide edited\n")
	fixture.writeArtifact("docs/notes.txt", "notes edited\n")

	stdout, stderr, err := fixture.runSync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if stdout != prefixed("Restored artifacts at "+headDisplay+"\n") {
		t.Fatalf("stdout = %q, want %q", stdout, prefixed("Restored artifacts at "+headDisplay+"\n"))
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	orphanManifests := fixture.readOrphanManifests()
	if len(orphanManifests) != 1 {
		t.Fatalf("orphan manifest count = %d, want 1", len(orphanManifests))
	}
	for _, storedOrphan := range orphanManifests {
		if len(storedOrphan.Artifacts) != 2 {
			t.Fatalf("orphan manifest entries = %d, want 2", len(storedOrphan.Artifacts))
		}
		if got := storedOrphan.Artifacts["docs/guide.md"]; got != hashContent([]byte("guide edited\n")) {
			t.Fatalf("guide orphan hash = %q, want %q", got, hashContent([]byte("guide edited\n")))
		}
		if got := storedOrphan.Artifacts["docs/notes.txt"]; got != hashContent([]byte("notes edited\n")) {
			t.Fatalf("notes orphan hash = %q, want %q", got, hashContent([]byte("notes edited\n")))
		}
	}
}

func TestSyncRestoreWithIdenticalLocalFileCreatesNoOrphanSnapshot(t *testing.T) {
	fixture := newSyncFixture(t, "repo-no-orphan-identical", "test.md")
	fixture.writeArtifact("test.md", "snapshot body\n")
	fixture.mustSync()

	if _, _, err := fixture.runSync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if got := fixture.orphanManifestCount(); got != 0 {
		t.Fatalf("orphan manifest count = %d, want 0", got)
	}
}

func TestSyncAllowsOverlappingArtifactsWithLaterEntryWinning(t *testing.T) {
	fixture := newSyncFixture(t, "repo-overlap", "docs", "docs/guide.md")
	fixture.writeArtifact("docs/guide.md", "guide v1\n")
	fixture.writeArtifact("docs/other.txt", "other v1\n")

	fixture.mustSync()

	storedManifest := fixture.readManifest(fixture.head())
	if len(storedManifest.Artifacts) != 2 {
		t.Fatalf("manifest entries = %d, want 2", len(storedManifest.Artifacts))
	}
	if got := storedManifest.Artifacts["docs/guide.md"]; got != hashContent([]byte("guide v1\n")) {
		t.Fatalf("manifest hash = %q, want %q", got, hashContent([]byte("guide v1\n")))
	}
}

func TestSyncSkipsSymlinksInsideConfiguredDirectory(t *testing.T) {
	fixture := newSyncFixture(t, "repo-symlink", "docs")
	fixture.writeArtifact("docs/guide.md", "guide v1\n")
	if err := os.Symlink("guide.md", fixture.artifactPath("docs/link.md")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	head := fixture.head()
	headDisplay := fixture.headDisplay()

	stdout, stderr, err := fixture.runSync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !strings.Contains(stdout, prefixed("warning: skipped symlink artifact docs/link.md\n")) {
		t.Fatalf("stdout = %q, want warning", stdout)
	}
	if !strings.Contains(stdout, prefixed("Created artifacts for "+headDisplay+"\n")) {
		t.Fatalf("stdout = %q, want create message", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	storedManifest := fixture.readManifest(head)
	if len(storedManifest.Artifacts) != 1 {
		t.Fatalf("manifest entries = %d, want 1", len(storedManifest.Artifacts))
	}
	if _, ok := storedManifest.Artifacts["docs/link.md"]; ok {
		t.Fatalf("manifest unexpectedly contains symlink entry")
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

func TestSyncEmptyManifestRestoresNothing(t *testing.T) {
	fixture := newSyncFixture(t, "repo-restore", "test.md")
	fixture.writeArtifact("test.md", "snapshot body\n")

	headDisplay := fixture.headDisplay()
	head := fixture.head()
	fixture.mustSync()
	fixture.writeManifest(head, manifest{
		Version:   manifestVersion,
		Artifacts: map[string]string{},
	})
	fixture.writeArtifact("test.md", "edited body\n")

	stdout, stderr, err := fixture.runSync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if stdout != prefixed("Restored artifacts at "+headDisplay+"\n") {
		t.Fatalf("stdout = %q, want %q", stdout, prefixed("Restored artifacts at "+headDisplay+"\n"))
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if got := fixture.readArtifact("test.md"); got != "edited body\n" {
		t.Fatalf("artifact = %q, want %q", got, "edited body\n")
	}
}

func TestSyncEmptyManifestLeavesDirectoryArtifactsUntouched(t *testing.T) {
	fixture := newSyncFixture(t, "repo-dir-restore", "docs")
	fixture.writeArtifact("docs/guide.md", "snapshot body\n")

	headDisplay := fixture.headDisplay()
	head := fixture.head()
	fixture.mustSync()
	fixture.writeManifest(head, manifest{
		Version:   manifestVersion,
		Artifacts: map[string]string{},
	})
	fixture.writeArtifact("docs/guide.md", "edited body\n")

	stdout, stderr, err := fixture.runSync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if stdout != prefixed("Restored artifacts at "+headDisplay+"\n") {
		t.Fatalf("stdout = %q, want %q", stdout, prefixed("Restored artifacts at "+headDisplay+"\n"))
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if got := fixture.readArtifact("docs/guide.md"); got != "edited body\n" {
		t.Fatalf("artifact = %q, want %q", got, "edited body\n")
	}
}

func TestBuildRestorePlanEmptyManifest(t *testing.T) {
	fixture := newSyncFixture(t, "repo-plan-empty", "test.md")

	plan, err := buildRestorePlan(fixture.store, manifest{
		Version:   manifestVersion,
		Artifacts: map[string]string{},
	})
	if err != nil {
		t.Fatalf("buildRestorePlan() error = %v", err)
	}
	if len(plan.targets) != 0 {
		t.Fatalf("target count = %d, want 0", len(plan.targets))
	}
	if len(plan.orphanCandidates) != 0 {
		t.Fatalf("orphan candidate count = %d, want 0", len(plan.orphanCandidates))
	}
}

func TestBuildRestorePlanIdenticalLocalFileDoesNotCreateOrphanCandidate(t *testing.T) {
	fixture := newSyncFixture(t, "repo-plan-identical", "test.md")
	content := "snapshot body\n"
	hash := fixture.writeBlobWithHash(content)
	fixture.writeArtifact("test.md", content)

	var plan restorePlan
	var err error
	testutil.WithWorkingDir(t, fixture.repoDir, func() {
		plan, err = buildRestorePlan(fixture.store, manifest{
			Version: manifestVersion,
			Artifacts: map[string]string{
				"test.md": hash,
			},
		})
	})
	if err != nil {
		t.Fatalf("buildRestorePlan() error = %v", err)
	}
	if len(plan.targets) != 1 {
		t.Fatalf("target count = %d, want 1", len(plan.targets))
	}
	if len(plan.orphanCandidates) != 0 {
		t.Fatalf("orphan candidate count = %d, want 0", len(plan.orphanCandidates))
	}
	if got := string(plan.targets[0].content); got != content {
		t.Fatalf("target content = %q, want %q", got, content)
	}
}

func TestBuildRestorePlanDifferentLocalFileCreatesOrphanCandidate(t *testing.T) {
	fixture := newSyncFixture(t, "repo-plan-different", "test.md")
	targetContent := "snapshot body\n"
	currentContent := "edited body\n"
	hash := fixture.writeBlobWithHash(targetContent)
	fixture.writeArtifact("test.md", currentContent)

	var plan restorePlan
	var err error
	testutil.WithWorkingDir(t, fixture.repoDir, func() {
		plan, err = buildRestorePlan(fixture.store, manifest{
			Version: manifestVersion,
			Artifacts: map[string]string{
				"test.md": hash,
			},
		})
	})
	if err != nil {
		t.Fatalf("buildRestorePlan() error = %v", err)
	}
	if len(plan.targets) != 1 {
		t.Fatalf("target count = %d, want 1", len(plan.targets))
	}
	if len(plan.orphanCandidates) != 1 {
		t.Fatalf("orphan candidate count = %d, want 1", len(plan.orphanCandidates))
	}
	if got := string(plan.targets[0].content); got != targetContent {
		t.Fatalf("target content = %q, want %q", got, targetContent)
	}
	candidate := plan.orphanCandidates[0]
	if candidate.path != "test.md" {
		t.Fatalf("orphan candidate path = %q, want %q", candidate.path, "test.md")
	}
	if candidate.hash != hashContent([]byte(currentContent)) {
		t.Fatalf("orphan candidate hash = %q, want %q", candidate.hash, hashContent([]byte(currentContent)))
	}
	if got := string(candidate.content); got != currentContent {
		t.Fatalf("orphan candidate content = %q, want %q", got, currentContent)
	}
}

func TestBuildRestorePlanMissingLocalFileDoesNotCreateOrphanCandidate(t *testing.T) {
	fixture := newSyncFixture(t, "repo-plan-missing", "test.md")
	content := "snapshot body\n"
	hash := fixture.writeBlobWithHash(content)

	var plan restorePlan
	var err error
	testutil.WithWorkingDir(t, fixture.repoDir, func() {
		plan, err = buildRestorePlan(fixture.store, manifest{
			Version: manifestVersion,
			Artifacts: map[string]string{
				"test.md": hash,
			},
		})
	})
	if err != nil {
		t.Fatalf("buildRestorePlan() error = %v", err)
	}
	if len(plan.targets) != 1 {
		t.Fatalf("target count = %d, want 1", len(plan.targets))
	}
	if len(plan.orphanCandidates) != 0 {
		t.Fatalf("orphan candidate count = %d, want 0", len(plan.orphanCandidates))
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

func TestBuildRestorePlanInvalidManifestHashFails(t *testing.T) {
	fixture := newSyncFixture(t, "repo-plan-invalid-hash", "test.md")

	_, err := buildRestorePlan(fixture.store, manifest{
		Version: manifestVersion,
		Artifacts: map[string]string{
			"test.md": "",
		},
	})
	if err == nil {
		t.Fatal("buildRestorePlan() error = nil, want error")
	}
	if err.Error() != "invalid manifest hash for artifact: test.md" {
		t.Fatalf("buildRestorePlan() error = %q, want %q", err.Error(), "invalid manifest hash for artifact: test.md")
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

func TestBuildRestorePlanMissingBlobFails(t *testing.T) {
	fixture := newSyncFixture(t, "repo-plan-missing-blob", "test.md")

	_, err := buildRestorePlan(fixture.store, manifest{
		Version: manifestVersion,
		Artifacts: map[string]string{
			"test.md": hashContent([]byte("snapshot body\n")),
		},
	})
	if err == nil {
		t.Fatal("buildRestorePlan() error = nil, want error")
	}
	if err.Error() != "blob missing for artifact: test.md" {
		t.Fatalf("buildRestorePlan() error = %q, want %q", err.Error(), "blob missing for artifact: test.md")
	}
}

func TestBuildRestorePlanUnreadableCurrentArtifactFailsBeforeMutation(t *testing.T) {
	fixture := newSyncFixture(t, "repo-plan-unreadable", "test.md")
	originalContent := "snapshot body\n"
	hash := fixture.writeBlobWithHash(originalContent)
	if err := os.MkdirAll(fixture.artifactPath("test.md"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	var err error
	testutil.WithWorkingDir(t, fixture.repoDir, func() {
		_, err = buildRestorePlan(fixture.store, manifest{
			Version: manifestVersion,
			Artifacts: map[string]string{
				"test.md": hash,
			},
		})
	})
	if err == nil {
		t.Fatal("buildRestorePlan() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to read current artifact test.md:") {
		t.Fatalf("buildRestorePlan() error = %q, want path-specific read error", err.Error())
	}
	if _, statErr := os.Stat(fixture.artifactPath("test.md")); statErr != nil {
		t.Fatalf("Stat() error = %v", statErr)
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

func TestSyncNoArtifactsRunsCleanupFirst(t *testing.T) {
	fixture := newSyncFixture(t, "repo-empty-cleanup")
	fixture.seedOrphanManifests(21, time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC), time.Minute)

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
	if got := fixture.orphanManifestCount(); got != orphanRetentionCount {
		t.Fatalf("orphan manifest count = %d, want %d", got, orphanRetentionCount)
	}
}

func TestSyncFailsOnInvalidOrphanManifestBeforeSnapshotOrRestore(t *testing.T) {
	fixture := newSyncFixture(t, "repo-invalid-orphan")
	if err := fixture.store.ensureDirs(); err != nil {
		t.Fatalf("ensureDirs() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.store.orphansPath(), "bad.json"), []byte("{not-json\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	stdout, stderr, err := fixture.runSync()
	if err == nil {
		t.Fatal("Sync() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to load orphan manifest bad") {
		t.Fatalf("Sync() error = %q, want invalid orphan manifest error", err.Error())
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestCleanupOrphansKeepsNewestTwentyAndDeletesOnlyManifestFiles(t *testing.T) {
	fixture := newSyncFixture(t, "repo-cleanup")
	ids := fixture.seedOrphanManifests(22, time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC), time.Minute)
	oldestManifest := fixture.readOrphanManifest(ids[21])
	oldestBlobHash := oldestManifest.Artifacts["artifact-21.txt"]

	if err := cleanupOrphans(fixture.store); err != nil {
		t.Fatalf("cleanupOrphans() error = %v", err)
	}

	gotIDs := fixture.orphanManifestIDs()
	if len(gotIDs) != orphanRetentionCount {
		t.Fatalf("orphan manifest count = %d, want %d", len(gotIDs), orphanRetentionCount)
	}
	if slices.Contains(gotIDs, ids[20]) || slices.Contains(gotIDs, ids[21]) {
		t.Fatalf("cleanup kept stale orphan ids: %#v", gotIDs)
	}
	if _, err := os.Stat(fixture.store.blobPath(oldestBlobHash)); err != nil {
		t.Fatalf("blob stat error = %v, want blob retained", err)
	}
}

func TestCleanupOrphansTieBreaksByIDAscending(t *testing.T) {
	fixture := newSyncFixture(t, "repo-cleanup-tie")
	timestamp := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	ids := make([]string, 0, 22)
	for i := 21; i >= 0; i-- {
		id := fmt.Sprintf("orphan-%02d", i)
		fixture.writeOrphanManifest(id, manifest{
			Version:   manifestVersion,
			CreatedAt: manifestTimestamp(timestamp),
			Artifacts: map[string]string{
				fmt.Sprintf("artifact-%02d.txt", i): fixture.writeBlobWithHash(fmt.Sprintf("content-%02d\n", i)),
			},
		})
		ids = append(ids, id)
	}

	if err := cleanupOrphans(fixture.store); err != nil {
		t.Fatalf("cleanupOrphans() error = %v", err)
	}

	gotIDs := fixture.orphanManifestIDs()
	wantIDs := []string{
		"orphan-00", "orphan-01", "orphan-02", "orphan-03", "orphan-04",
		"orphan-05", "orphan-06", "orphan-07", "orphan-08", "orphan-09",
		"orphan-10", "orphan-11", "orphan-12", "orphan-13", "orphan-14",
		"orphan-15", "orphan-16", "orphan-17", "orphan-18", "orphan-19",
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("cleanup kept ids = %#v, want %#v", gotIDs, wantIDs)
	}
}

func TestLoadManifestRejectsInvalidCreatedAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	writeManifestFile(t, path, manifest{
		Version:   manifestVersion,
		CreatedAt: "not-a-time",
		Artifacts: map[string]string{},
	})

	_, err := loadManifest(path)
	if err == nil {
		t.Fatal("loadManifest() error = nil, want error")
	}
	if err.Error() != "invalid manifest created_at: parsing time \"not-a-time\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"not-a-time\" as \"2006\"" {
		t.Fatalf("loadManifest() error = %q", err.Error())
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

func TestApplyRestorePlanUsesPlannedContent(t *testing.T) {
	fixture := newSyncFixture(t, "repo-apply-plan", "test.md")
	hash := fixture.writeBlobWithHash("original blob content\n")

	var plan restorePlan
	var err error
	testutil.WithWorkingDir(t, fixture.repoDir, func() {
		plan, err = buildRestorePlan(fixture.store, manifest{
			Version: manifestVersion,
			Artifacts: map[string]string{
				"test.md": hash,
			},
		})
	})
	if err != nil {
		t.Fatalf("buildRestorePlan() error = %v", err)
	}

	if err := os.WriteFile(fixture.store.blobPath(hash), []byte("changed after planning\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	testutil.WithWorkingDir(t, fixture.repoDir, func() {
		err = applyRestorePlan(plan)
	})
	if err != nil {
		t.Fatalf("applyRestorePlan() error = %v", err)
	}
	if got := fixture.readArtifact("test.md"); got != "original blob content\n" {
		t.Fatalf("artifact = %q, want %q", got, "original blob content\n")
	}
}

func TestLoadOrphanManifestsRejectsMissingCreatedAt(t *testing.T) {
	fixture := newSyncFixture(t, "repo-orphan-missing-created")
	fixture.writeOrphanManifest("orphan-1", manifest{
		Version:   manifestVersion,
		Artifacts: map[string]string{},
	})

	_, err := loadOrphanManifests(fixture.store)
	if err == nil {
		t.Fatal("loadOrphanManifests() error = nil, want error")
	}
	if err.Error() != "failed to load orphan manifest orphan-1: missing manifest created_at" {
		t.Fatalf("loadOrphanManifests() error = %q", err.Error())
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

func (f syncFixture) writeBlobWithHash(content string) string {
	f.t.Helper()

	hash := hashContent([]byte(content))
	if err := f.store.ensureDirs(); err != nil {
		f.t.Fatalf("ensureDirs() error = %v", err)
	}
	if err := writeBlob(f.store, hash, []byte(content)); err != nil {
		f.t.Fatalf("writeBlob() error = %v", err)
	}
	return hash
}

func (f syncFixture) readManifest(commit string) manifest {
	f.t.Helper()
	return readManifest(f.t, f.store.manifestPath(commit))
}

func (f syncFixture) readOrphanManifest(id string) manifest {
	f.t.Helper()
	return readManifest(f.t, f.store.orphanManifestPath(id))
}

func (f syncFixture) readOrphanManifests() map[string]manifest {
	f.t.Helper()

	paths, err := f.store.orphanManifestPaths()
	if err != nil {
		f.t.Fatalf("orphanManifestPaths() error = %v", err)
	}

	out := make(map[string]manifest, len(paths))
	for _, path := range paths {
		out[orphanIDFromPath(path)] = readManifest(f.t, path)
	}
	return out
}

func (f syncFixture) orphanManifestCount() int {
	f.t.Helper()
	return len(f.readOrphanManifests())
}

func (f syncFixture) orphanManifestIDs() []string {
	f.t.Helper()

	paths, err := f.store.orphanManifestPaths()
	if err != nil {
		f.t.Fatalf("orphanManifestPaths() error = %v", err)
	}

	ids := make([]string, 0, len(paths))
	for _, path := range paths {
		ids = append(ids, orphanIDFromPath(path))
	}
	return ids
}

func (f syncFixture) writeManifest(commit string, storedManifest manifest) {
	f.t.Helper()
	writeManifestFile(f.t, f.store.manifestPath(commit), storedManifest)
}

func (f syncFixture) writeOrphanManifest(id string, storedManifest manifest) {
	f.t.Helper()
	if err := f.store.ensureDirs(); err != nil {
		f.t.Fatalf("ensureDirs() error = %v", err)
	}
	writeManifestFile(f.t, f.store.orphanManifestPath(id), storedManifest)
}

func (f syncFixture) seedOrphanManifests(count int, newest time.Time, step time.Duration) []string {
	f.t.Helper()

	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("orphan-%02d", i)
		ids = append(ids, id)
		f.writeOrphanManifest(id, manifest{
			Version:   manifestVersion,
			CreatedAt: manifestTimestamp(newest.Add(-time.Duration(i) * step)),
			Artifacts: map[string]string{
				fmt.Sprintf("artifact-%d.txt", i): f.writeBlobWithHash(fmt.Sprintf("blob-%d\n", i)),
			},
		})
	}
	return ids
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
