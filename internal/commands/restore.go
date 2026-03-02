package commands

import (
	"bufio"
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
	"verti/internal/reporting"
	"verti/internal/restoremode"
	"verti/internal/restoreplan"
	"verti/internal/snapshots"
)

const restoreOrphanFlag = "--orphan"
const restoreSkippedOutOfSyncMessage = "verti: restore skipped. Code and artifacts are now out of sync."

type restoreDecision int

const (
	restoreDecisionProceed restoreDecision = iota
	restoreDecisionDeclined
	restoreDecisionNoTTY
	restoreDecisionSkip
)

type restoreDecisionContext struct {
	TargetSHA    string
	OrphanID     string
	OrphanPath   string
	SnapshotPath string
}

type restoreApplyContext struct {
	TargetSHA    string
	SnapshotPath string
	OrphanID     string
	OrphanPath   string
	Plan         []restoreplan.Operation
	Manifest     []artifacts.ManifestEntry
	RepoRoot     string
	StoreRoot    string
	RepoID       string
	WorktreeID   string
}

var beforeRestoreDecisionHook = func(restoreDecisionContext) error { return nil }
var applyRestorePlanHook = applyRestorePlan
var openPromptTTY = func() (io.ReadWriteCloser, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}
var promptRestoreConfirmationFn = promptRestoreConfirmation

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
		orphanPath, found, err := snapshots.FindOrphanSnapshot(scopeDir, target)
		if err != nil {
			return fmt.Errorf("lookup orphan snapshot %q: %w", target, err)
		}
		if !found {
			return nil
		}

		manifest, err := loadSnapshotManifest(orphanPath)
		if err != nil {
			return err
		}
		meta, err := loadSnapshotMeta(orphanPath)
		if err != nil {
			return err
		}

		currentPaths, err := currentPresentArtifactPaths(repoRoot, cfg.Artifacts)
		if err != nil {
			return err
		}
		plan, err := restoreplan.BuildPlan(repoRoot, manifest.Entries, currentPaths)
		if err != nil {
			return reporting.Wrap(reporting.ClassRestore, "build restore plan", err)
		}

		targetLabel := "orphan:" + target
		if strings.TrimSpace(meta.TriggeringCheckoutSHA) != "" {
			targetLabel = meta.TriggeringCheckoutSHA
		}

		if err := applyRestorePlanHook(restoreApplyContext{
			TargetSHA:    targetLabel,
			SnapshotPath: orphanPath,
			OrphanID:     target,
			OrphanPath:   orphanPath,
			Plan:         plan,
			Manifest:     manifest.Entries,
			RepoRoot:     repoRoot,
			StoreRoot:    storeRoot,
			RepoID:       cfg.RepoID,
			WorktreeID:   worktreeID.WorktreeID,
		}); err != nil {
			return reporting.Wrap(reporting.ClassRestore, "apply restore plan", err)
		}
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
		meta, err := loadSnapshotMeta(snapshotPath)
		if err != nil {
			return err
		}
		currentPaths, err := currentPresentArtifactPaths(repoRoot, cfg.Artifacts)
		if err != nil {
			return err
		}
		plan, err := restoreplan.BuildPlan(repoRoot, manifest.Entries, currentPaths)
		if err != nil {
			return reporting.Wrap(reporting.ClassRestore, "build restore plan", err)
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

		restoreMode, err := resolveEffectiveRestoreMode(cfg.RestoreMode, stderr)
		if err != nil {
			return err
		}

		decision, err := shouldProceedWithRestore(restoreMode, target, meta.Branch)
		if err != nil {
			return err
		}
		if decision == restoreDecisionSkip {
			return nil
		}
		if decision == restoreDecisionNoTTY {
			warnf(stderr, "verti: no interactive TTY; skipping restore. To apply manually, run: verti restore %s", target)
			return nil
		}
		if decision == restoreDecisionDeclined {
			warnf(stderr, restoreSkippedOutOfSyncMessage)
			return nil
		}

		if err := applyRestorePlanHook(restoreApplyContext{
			TargetSHA:    target,
			SnapshotPath: snapshotPath,
			OrphanID:     orphanID,
			OrphanPath:   orphanPath,
			Plan:         plan,
			Manifest:     manifest.Entries,
			RepoRoot:     repoRoot,
			StoreRoot:    storeRoot,
			RepoID:       cfg.RepoID,
			WorktreeID:   worktreeID.WorktreeID,
		}); err != nil {
			return reporting.Wrap(reporting.ClassRestore, "apply restore plan", err)
		}

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
		return config.Config{}, reporting.Wrap(reporting.ClassConfig, "load config", err)
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

func loadSnapshotMeta(snapshotPath string) (snapshots.Meta, error) {
	raw, err := os.ReadFile(filepath.Join(snapshotPath, "meta.json"))
	if err != nil {
		return snapshots.Meta{}, fmt.Errorf("read snapshot meta at %q: %w", snapshotPath, err)
	}

	var meta snapshots.Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return snapshots.Meta{}, fmt.Errorf("parse snapshot meta at %q: %w", snapshotPath, err)
	}

	return meta, nil
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

func shouldProceedWithRestore(mode, targetSHA, branch string) (restoreDecision, error) {
	switch mode {
	case config.RestoreModeForce:
		return restoreDecisionProceed, nil
	case config.RestoreModeSkip:
		return restoreDecisionSkip, nil
	case config.RestoreModePrompt:
	default:
		return restoreDecisionDeclined, fmt.Errorf("unsupported restore mode %q", mode)
	}

	tty, err := openPromptTTY()
	if err != nil {
		return restoreDecisionNoTTY, nil
	}
	defer tty.Close()

	confirmed, err := promptRestoreConfirmationFn(tty, targetSHA, branch)
	if err != nil {
		return restoreDecisionDeclined, fmt.Errorf("prompt restore confirmation: %w", err)
	}
	if !confirmed {
		return restoreDecisionDeclined, nil
	}
	return restoreDecisionProceed, nil
}

func promptRestoreConfirmation(tty io.ReadWriter, targetSHA, branch string) (bool, error) {
	if strings.TrimSpace(branch) == "" {
		fmt.Fprintf(tty, "verti: snapshot found for %s.\n", targetSHA)
	} else {
		fmt.Fprintf(tty, "verti: snapshot found for %s (branch: %s).\n", targetSHA, branch)
	}
	fmt.Fprintln(tty, "This will delete un-snapshotted files in configured directories.")
	fmt.Fprint(tty, "Restore artifacts? [y/N] ")

	response, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("read restore prompt response: %w", err)
	}

	answer := strings.ToLower(strings.TrimSpace(response))
	return answer == "y" || answer == "yes", nil
}

func resolveEffectiveRestoreMode(configMode string, warnings io.Writer) (string, error) {
	env := map[string]string{}
	if raw, ok := os.LookupEnv("VERTI_RESTORE_MODE"); ok {
		env["VERTI_RESTORE_MODE"] = raw
	}

	mode, err := restoremode.Resolve(configMode, env, warnings)
	if err != nil {
		return "", fmt.Errorf("resolve restore mode: %w", err)
	}
	return mode, nil
}
