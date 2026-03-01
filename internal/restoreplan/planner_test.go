package restoreplan

import (
	"reflect"
	"strings"
	"testing"

	"verti/internal/artifacts"
)

func TestBuildPlanRejectsPathEscapeEntries(t *testing.T) {
	repoRoot := t.TempDir()

	_, err := BuildPlan(repoRoot, []artifacts.ManifestEntry{
		{Path: "../outside.txt", Kind: artifacts.ArtifactKindFile, Status: artifacts.ArtifactStatusPresent},
	}, nil)
	if err == nil {
		t.Fatalf("BuildPlan() expected path-escape validation error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "escape") {
		t.Fatalf("expected path-escape error, got %v", err)
	}
}

func TestBuildPlanRejectsFileDirTargetCollisions(t *testing.T) {
	repoRoot := t.TempDir()

	_, err := BuildPlan(repoRoot, []artifacts.ManifestEntry{
		{Path: "notes", Kind: artifacts.ArtifactKindFile, Status: artifacts.ArtifactStatusPresent},
		{Path: "notes/daily.md", Kind: artifacts.ArtifactKindFile, Status: artifacts.ArtifactStatusPresent},
	}, nil)
	if err == nil {
		t.Fatalf("BuildPlan() expected collision validation error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "collision") {
		t.Fatalf("expected collision error, got %v", err)
	}
}

func TestBuildPlanIsDeterministicAcrossRuns(t *testing.T) {
	repoRoot := t.TempDir()

	target := []artifacts.ManifestEntry{
		{Path: "md/sub", Kind: artifacts.ArtifactKindDir, Status: artifacts.ArtifactStatusPresent},
		{Path: "b.txt", Kind: artifacts.ArtifactKindFile, Status: artifacts.ArtifactStatusPresent},
		{Path: "md", Kind: artifacts.ArtifactKindDir, Status: artifacts.ArtifactStatusPresent},
		{Path: "lnk", Kind: artifacts.ArtifactKindSymlink, LinkTarget: "b.txt", Status: artifacts.ArtifactStatusPresent},
		{Path: "old.txt", Kind: artifacts.ArtifactKindMissing, Status: artifacts.ArtifactStatusMissing},
		{Path: "md/a.txt", Kind: artifacts.ArtifactKindFile, Status: artifacts.ArtifactStatusPresent},
	}
	current := []string{"old.txt", "md", "md/old.md", "b.txt", "lnk", "z.tmp"}

	first, err := BuildPlan(repoRoot, target, current)
	if err != nil {
		t.Fatalf("BuildPlan(first) error = %v", err)
	}
	second, err := BuildPlan(repoRoot, target, current)
	if err != nil {
		t.Fatalf("BuildPlan(second) error = %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("BuildPlan() not deterministic across runs")
	}

	want := []string{
		"mkdir md",
		"mkdir md/sub",
		"write_file b.txt",
		"write_file md/a.txt",
		"write_symlink lnk",
		"remove md/old.md",
		"remove z.tmp",
		"remove old.txt",
	}
	got := opStrings(first)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected plan order:\n got  %#v\n want %#v", got, want)
	}
}

func opStrings(ops []Operation) []string {
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		out = append(out, string(op.Type)+" "+op.Path)
	}
	return out
}
