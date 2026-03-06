package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"verti/internal/artifacts"
	"verti/internal/cli"
	"verti/internal/config"
	"verti/internal/git"
	"verti/internal/identity"
	"verti/internal/logging"
	"verti/internal/restoremode"
	"verti/internal/restoreplan"
	"verti/internal/snapshots"
)

const (
	syncDebouncedFlag              = "--debounced"
	syncDebounceWindow             = 500 * time.Millisecond
	syncStateDirName               = "verti-sync"
	restoreSkippedOutOfSyncMessage = "verti: restore skipped. Code and artifacts are now out of sync."
	orphanRetentionMax             = 20
)

type syncOptions struct {
	Debounced bool
}

type syncRepoState struct {
	CommitSHA      string
	BranchIdentity string
	SnapshotID     string
}

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

// RunSync reconciles artifact state for the current worktree state.
func RunSync(workingDir string, args []string) error {
	return runSync(workingDir, args, os.Stderr)
}

func runSync(workingDir string, args []string, stderr io.Writer) error {
	opts, err := parseSyncArgs(args)
	if err != nil {
		return err
	}

	if opts.Debounced {
		return runSyncDebounced(workingDir, stderr)
	}
	return runSyncImmediate(workingDir, stderr)
}

func parseSyncArgs(args []string) (syncOptions, error) {
	if len(args) == 0 {
		return syncOptions{}, nil
	}
	if len(args) == 1 && args[0] == syncDebouncedFlag {
		return syncOptions{Debounced: true}, nil
	}
	return syncOptions{}, &cli.UsageError{Message: "sync accepts no positional args; use --debounced for hook-internal trailing debounce"}
}

func runSyncDebounced(workingDir string, stderr io.Writer) error {
	ctx, err := LoadContext(workingDir, []ContextField{
		ContextFieldConfig,
		ContextFieldWorktreeIdentity,
	})
	if err != nil {
		return err
	}
	if !ctx.Config.Value.Enabled {
		return nil
	}

	stateDir := filepath.Join(ctx.Git.CommonGitDir, syncStateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create sync state dir %q: %w", stateDir, err)
	}

	token := uuid.NewString()
	tokenPath := debounceTokenPath(stateDir, ctx.Worktree.WorktreeID)
	if err := writeAtomicFile(tokenPath, []byte(token+"\n"), 0o644); err != nil {
		return fmt.Errorf("write debounce token %q: %w", tokenPath, err)
	}

	time.Sleep(syncDebounceWindow)

	latest, err := readTrimmedFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read debounce token %q: %w", tokenPath, err)
	}
	if latest != token {
		return nil
	}

	unlockDebounce, err := acquireFileLock(debounceLockPath(stateDir, ctx.Worktree.WorktreeID))
	if err != nil {
		return err
	}
	defer unlockDebounce()

	latest, err = readTrimmedFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("re-read debounce token %q: %w", tokenPath, err)
	}
	if latest != token {
		return nil
	}

	return runSyncImmediate(workingDir, stderr)
}

func runSyncImmediate(workingDir string, stderr io.Writer) error {
	ctx, err := LoadContext(workingDir, []ContextField{
		ContextFieldRepoRoot,
		ContextFieldConfig,
		ContextFieldStoreRoot,
		ContextFieldWorktreeIdentity,
		ContextFieldStorePaths,
	})
	if err != nil {
		return err
	}
	if !ctx.Config.Value.Enabled {
		return nil
	}

	stateDir := filepath.Join(ctx.Git.CommonGitDir, syncStateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create sync state dir %q: %w", stateDir, err)
	}

	unlockApply, err := acquireFileLock(syncApplyLockPath(stateDir, ctx.Worktree.WorktreeID))
	if err != nil {
		return err
	}
	defer unlockApply()

	state, err := resolveCurrentSyncState(workingDir)
	if err != nil {
		return err
	}

	if err := reconcileCurrentState(ctx, state, stderr); err != nil {
		return err
	}

	return nil
}

func resolveCurrentSyncState(workingDir string) (syncRepoState, error) {
	headSHA, err := git.HeadSHA(workingDir)
	if err != nil {
		return syncRepoState{}, fmt.Errorf("resolve HEAD sha: %w", err)
	}

	branch, err := git.CurrentBranch(workingDir)
	if err != nil {
		return syncRepoState{}, fmt.Errorf("resolve current branch: %w", err)
	}

	branchIdentity := strings.TrimSpace(branch)
	if branchIdentity == "" {
		branchIdentity = snapshots.DetachedBranchIdentity
	}

	snapshotID, err := snapshots.SnapshotID(branchIdentity, headSHA)
	if err != nil {
		return syncRepoState{}, fmt.Errorf("derive snapshot id: %w", err)
	}

	return syncRepoState{
		CommitSHA:      headSHA,
		BranchIdentity: branchIdentity,
		SnapshotID:     snapshotID,
	}, nil
}

func reconcileCurrentState(ctx Context, state syncRepoState, stderr io.Writer) error {
	snapshotPath, found, err := snapshots.FindSnapshot(ctx.Paths.WorktreeScopeDir, state.SnapshotID)
	if err != nil {
		return fmt.Errorf("lookup snapshot %q: %w", state.SnapshotID, err)
	}

	if found {
		return restoreForState(ctx, state, snapshotPath, stderr)
	}

	return publishForState(ctx, state, stderr)
}

func publishForState(ctx Context, state syncRepoState, stderr io.Writer) error {
	cfg := ctx.Config.Value
	repoRoot := ctx.Git.RepoRoot
	manifestEntries, err := artifacts.BuildManifestEntries(repoRoot, cfg.Artifacts)
	if err != nil {
		return fmt.Errorf("build artifact manifest entries: %w", err)
	}

	writeManifestObjects(repoRoot, ctx.Store.Root, cfg.RepoID, cfg.MaxFileSizeMB, manifestEntries, stderr)

	worktreeID := *ctx.Worktree
	meta := snapshots.Meta{
		CommitSHA:               state.CommitSHA,
		Branch:                  state.BranchIdentity,
		WorktreeID:              worktreeID.WorktreeID,
		WorktreePathFingerprint: worktreeID.WorktreePathFingerprint,
	}

	if _, err := snapshots.PublishSnapshot(ctx.Paths.WorktreeScopeDir, state.SnapshotID, manifestEntries, meta); err != nil {
		return fmt.Errorf("publish snapshot: %w", err)
	}
	return nil
}

func restoreForState(ctx Context, state syncRepoState, snapshotPath string, stderr io.Writer) error {
	cfg := ctx.Config.Value
	repoRoot := ctx.Git.RepoRoot
	storeRoot := ctx.Store.Root
	worktreeID := *ctx.Worktree

	manifest, err := loadSnapshotManifest(snapshotPath)
	if err != nil {
		return err
	}
	meta, err := loadSnapshotMeta(snapshotPath)
	if err != nil {
		return err
	}

	currentEntries, err := currentArtifactManifestEntries(repoRoot, cfg.Artifacts)
	if err != nil {
		return err
	}
	currentPaths := presentArtifactPaths(currentEntries)
	plan, err := restoreplan.BuildPlan(repoRoot, manifest.Entries, currentPaths)
	if err != nil {
		return fmt.Errorf("build restore plan: %w", err)
	}
	if !restoreWouldChangeState(manifest.Entries, currentEntries) {
		return nil
	}

	restoreMode, err := resolveEffectiveRestoreMode(cfg.RestoreMode, stderr)
	if err != nil {
		return err
	}

	decision, err := shouldProceedWithRestore(restoreMode, state.CommitSHA, meta.Branch)
	if err != nil {
		return err
	}
	if decision == restoreDecisionSkip {
		return nil
	}
	if decision == restoreDecisionNoTTY {
		if stderr != nil {
			fmt.Fprintln(stderr, "verti: no interactive TTY; skipping restore. To apply manually, run: VERTI_RESTORE_MODE=force verti sync")
		}
		return nil
	}
	if decision == restoreDecisionDeclined {
		if stderr != nil {
			fmt.Fprintln(stderr, restoreSkippedOutOfSyncMessage)
		}
		return nil
	}

	orphanID, orphanPath, err := createPreRestoreOrphanSnapshot(
		repoRoot,
		ctx.Paths.WorktreeScopeDir,
		storeRoot,
		cfg,
		worktreeID,
		state.CommitSHA,
		stderr,
	)
	if err != nil {
		return fmt.Errorf("create pre-restore orphan snapshot: %w", err)
	}
	if err := pruneOrphanSnapshots(ctx.Paths.WorktreeScopeDir, orphanRetentionMax); err != nil {
		logging.Warnf(stderr, "warning: unable to prune old orphan snapshots: %v", err)
	}

	if err := beforeRestoreDecisionHook(restoreDecisionContext{
		TargetSHA:    state.CommitSHA,
		OrphanID:     orphanID,
		OrphanPath:   orphanPath,
		SnapshotPath: snapshotPath,
	}); err != nil {
		return fmt.Errorf("run pre-decision restore hook: %w", err)
	}

	if err := applyRestorePlanHook(restoreApplyContext{
		TargetSHA:    state.CommitSHA,
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
		return fmt.Errorf("apply restore plan: %w", err)
	}

	return nil
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

func currentArtifactManifestEntries(repoRoot string, configured []string) ([]artifacts.ManifestEntry, error) {
	entries, err := artifacts.BuildManifestEntries(repoRoot, configured)
	if err != nil {
		return nil, fmt.Errorf("build current artifact manifest for restore: %w", err)
	}
	return entries, nil
}

func presentArtifactPaths(entries []artifacts.ManifestEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Status == artifacts.ArtifactStatusPresent {
			out = append(out, e.Path)
		}
	}
	return out
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

type orphanSnapshotInfo struct {
	Name      string
	Path      string
	CreatedAt time.Time
}

func pruneOrphanSnapshots(scopeDir string, keep int) error {
	if keep <= 0 {
		return nil
	}

	orphansDir := filepath.Join(scopeDir, "orphans")
	entries, err := os.ReadDir(orphansDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read orphans directory %q: %w", orphansDir, err)
	}

	var infos []orphanSnapshotInfo
	var errs error
	for _, entry := range entries {
		if !entry.IsDir() || snapshots.IsInternalCollectionDir(entry.Name()) {
			continue
		}

		createdAt, err := orphanSnapshotCreatedAt(orphansDir, entry)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("resolve orphan timestamp for %q: %w", entry.Name(), err))
			continue
		}

		infos = append(infos, orphanSnapshotInfo{
			Name:      entry.Name(),
			Path:      filepath.Join(orphansDir, entry.Name()),
			CreatedAt: createdAt,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		if !infos[i].CreatedAt.Equal(infos[j].CreatedAt) {
			return infos[i].CreatedAt.After(infos[j].CreatedAt)
		}
		return infos[i].Name > infos[j].Name
	})

	for i := keep; i < len(infos); i++ {
		if err := os.RemoveAll(infos[i].Path); err != nil {
			errs = errors.Join(errs, fmt.Errorf("remove orphan snapshot %q: %w", infos[i].Path, err))
		}
	}

	return errs
}

func orphanSnapshotCreatedAt(orphansDir string, entry os.DirEntry) (time.Time, error) {
	meta, err := loadSnapshotMeta(filepath.Join(orphansDir, entry.Name()))
	if err == nil {
		createdAt, parseErr := time.Parse(time.RFC3339, meta.CreatedAt)
		if parseErr == nil {
			return createdAt.UTC(), nil
		}
	}

	info, err := entry.Info()
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime().UTC(), nil
}

func restoreWouldChangeState(targetEntries, currentEntries []artifacts.ManifestEntry) bool {
	targetPresent := make(map[string]artifacts.ManifestEntry)
	for _, entry := range targetEntries {
		if entry.Status == artifacts.ArtifactStatusPresent {
			targetPresent[entry.Path] = entry
		}
	}

	currentPresent := make(map[string]artifacts.ManifestEntry)
	for _, entry := range currentEntries {
		if entry.Status == artifacts.ArtifactStatusPresent {
			currentPresent[entry.Path] = entry
		}
	}

	if len(targetPresent) != len(currentPresent) {
		return true
	}

	for path, target := range targetPresent {
		current, ok := currentPresent[path]
		if !ok {
			return true
		}
		if target.Kind != current.Kind {
			return true
		}

		switch target.Kind {
		case artifacts.ArtifactKindFile:
			if target.Hash != current.Hash || target.Mode != current.Mode {
				return true
			}
		case artifacts.ArtifactKindSymlink:
			if target.LinkTarget != current.LinkTarget {
				return true
			}
		default:
			// directory presence is sufficient for no-op detection.
		}
	}

	return false
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

func debounceTokenPath(stateDir, worktreeID string) string {
	return filepath.Join(stateDir, worktreeID+".debounce.token")
}

func debounceLockPath(stateDir, worktreeID string) string {
	return filepath.Join(stateDir, worktreeID+".debounce.lock")
}

func syncApplyLockPath(stateDir, worktreeID string) string {
	return filepath.Join(stateDir, worktreeID+".sync.lock")
}

func readTrimmedFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("open temp file %q: %w", tmpPath, err)
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file %q: %w", tmpPath, err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file %q: %w", tmpPath, err)
	}

	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file %q to %q: %w", tmpPath, path, err)
	}

	return nil
}

func acquireFileLock(path string) (func(), error) {
	lockFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file %q: %w", path, err)
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("acquire lock %q: %w", path, err)
	}

	return func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}, nil
}
