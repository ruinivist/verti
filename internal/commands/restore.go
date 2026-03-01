package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"verti/internal/artifacts"
	"verti/internal/cli"
	"verti/internal/config"
	"verti/internal/git"
	"verti/internal/identity"
	"verti/internal/restoreplan"
	"verti/internal/snapshots"
)

const restoreOrphanFlag = "--orphan"

type restoreDecisionContext struct {
	TargetSHA    string
	OrphanID     string
	OrphanPath   string
	SnapshotPath string
}

var beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }

// RunRestore resolves a restore target and no-ops when no snapshot is found.
func RunRestore(workingDir string, args []string) error {
	return runRestore(workingDir, args, os.Stderr)
}

func runRestore(workingDir string, args []string, stderr io.Writer) error {
	target, targetKind, err := parseRestoreArgs(args)
	if err != nil {
		return err
	}
	repoRoot, err := git.RepoRoot(workingDir)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	cfg, err := loadRepoConfig(workingDir)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return nil
	}
	if cfg.RepoID == "" {
		return fmt.Errorf("config missing repo_id; run `verti init`")
	}

	storeRoot, err := expandStoreRoot(cfg.StoreRoot)
	if err != nil {
		return err
	}

	worktreeID, err := identity.ResolveWorktreeIdentity(workingDir)
	if err != nil {
		return fmt.Errorf("resolve worktree identity: %w", err)
	}

	scopeDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", worktreeID.WorktreeID)

	switch targetKind {
	case "orphan":
		// Orphan restore behavior is implemented in a later task.
		_ = target
		_ = stderr
		return nil
	case "snapshot":
		snapshotPath, found, err := snapshots.FindSnapshot(scopeDir, target)
		if err != nil {
			return fmt.Errorf("lookup snapshot %q: %w", target, err)
		}
		if !found {
			return nil
		}

		manifest, err := loadSnapshotManifest(snapshotPath)
		if err != nil {
			return err
		}
		currentPaths, err := currentPresentArtifactPaths(repoRoot, cfg.Artifacts)
		if err != nil {
			return err
		}
		if _, err := restoreplan.BuildPlan(repoRoot, manifest.Entries, currentPaths); err != nil {
			return fmt.Errorf("build restore plan for snapshot %q: %w", target, err)
		}

		orphanID, orphanPath, err := createPreRestoreOrphanSnapshot(repoRoot, scopeDir, storeRoot, cfg, worktreeID, target, stderr)
		if err != nil {
			return fmt.Errorf("create pre-restore orphan snapshot: %w", err)
		}

		if err := beforeRestoreDecisionHook(restoreDecisionContext{
			TargetSHA:    target,
			OrphanID:     orphanID,
			OrphanPath:   orphanPath,
			SnapshotPath: snapshotPath,
		}); err != nil {
			return fmt.Errorf("run pre-decision restore hook: %w", err)
		}

		// Restore apply pipeline is implemented in later tasks.
		_ = stderr
		return nil
	default:
		return fmt.Errorf("unsupported restore target kind %q", targetKind)
	}
}

func parseRestoreArgs(args []string) (target string, targetKind string, err error) {
	if len(args) == 0 {
		return "", "", &cli.UsageError{Message: "restore requires a target SHA argument or --orphan <id>"}
	}

	if args[0] == restoreOrphanFlag {
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return "", "", &cli.UsageError{Message: "restore --orphan requires an orphan id"}
		}
		return args[1], "orphan", nil
	}

	if len(args) != 1 {
		return "", "", &cli.UsageError{Message: "restore accepts exactly one target SHA (or use --orphan <id>)"}
	}
	if strings.HasPrefix(args[0], "-") {
		return "", "", &cli.UsageError{Message: fmt.Sprintf("unknown restore option: %s", args[0])}
	}

	return args[0], "snapshot", nil
}

func loadRepoConfig(workingDir string) (config.Config, error) {
	commonGitDir, err := git.CommonGitDir(workingDir)
	if err != nil {
		return config.Config{}, fmt.Errorf("resolve common git dir: %w", err)
	}

	cfgPath := filepath.Join(commonGitDir, "verti.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}

	return cfg, nil
}

func loadSnapshotManifest(snapshotPath string) (snapshots.Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(snapshotPath, "manifest.json"))
	if err != nil {
		return snapshots.Manifest{}, fmt.Errorf("read snapshot manifest at %q: %w", snapshotPath, err)
	}

	var manifest snapshots.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return snapshots.Manifest{}, fmt.Errorf("parse snapshot manifest at %q: %w", snapshotPath, err)
	}

	return manifest, nil
}

func currentPresentArtifactPaths(repoRoot string, configured []string) ([]string, error) {
	entries, err := artifacts.BuildManifestEntries(repoRoot, configured)
	if err != nil {
		return nil, fmt.Errorf("build current artifact manifest for restore planning: %w", err)
	}

	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Status == artifacts.ArtifactStatusPresent {
			out = append(out, e.Path)
		}
	}
	return out, nil
}

func createPreRestoreOrphanSnapshot(repoRoot, scopeDir, storeRoot string, cfg config.Config, worktreeID identity.WorktreeIdentity, targetSHA string, stderr io.Writer) (string, string, error) {
	entries, err := artifacts.BuildManifestEntries(repoRoot, cfg.Artifacts)
	if err != nil {
		return "", "", fmt.Errorf("build current artifact manifest for orphan snapshot: %w", err)
	}

	writeManifestObjects(repoRoot, storeRoot, cfg.RepoID, cfg.MaxFileSizeMB, entries, stderr)

	orphanID := uuid.NewString()
	meta := snapshots.Meta{
		WorktreeID:              worktreeID.WorktreeID,
		WorktreePathFingerprint: worktreeID.WorktreePathFingerprint,
		SnapshotKind:            snapshots.SnapshotKindOrphan,
		OrphanID:                orphanID,
		TriggeringCheckoutSHA:   targetSHA,
	}
	orphanPath, err := snapshots.PublishOrphanSnapshot(scopeDir, orphanID, entries, meta)
	if err != nil {
		return "", "", fmt.Errorf("publish orphan snapshot %q: %w", orphanID, err)
	}

	return orphanID, orphanPath, nil
}
