package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"verti/internal/cli"
	"verti/internal/identity"
	"verti/internal/snapshots"
)

type listRow struct {
	Commit                string
	Branch                string
	CreatedAt             string
	Kind                  string
	OrphanID              string
	TriggeringCheckoutSHA string
}

// RunList prints snapshots for the current worktree.
func RunList(workingDir string, args []string) error {
	return runList(workingDir, args, os.Stdout)
}

func runList(workingDir string, args []string, stdout io.Writer) error {
	includeOrphans, err := parseListArgs(args)
	if err != nil {
		return err
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

	worktreeRoot := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", worktreeID.WorktreeID)
	rows, err := loadListRows(filepath.Join(worktreeRoot, "snapshots"), filepath.Join(worktreeRoot, "orphans"), includeOrphans)
	if err != nil {
		return err
	}

	return writeListRows(stdout, rows, includeOrphans)
}

func parseListArgs(args []string) (includeOrphans bool, err error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--orphans" {
		return true, nil
	}
	return false, &cli.UsageError{Message: "list accepts no positional args; use --orphans to include orphan snapshots"}
}

func loadListRows(snapshotsDir, orphansDir string, includeOrphans bool) ([]listRow, error) {
	entries, err := os.ReadDir(snapshotsDir)
	if os.IsNotExist(err) {
		entries = nil
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshots directory %q: %w", snapshotsDir, err)
	}

	rows := make([]listRow, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if isInternalCollectionDir(entry.Name()) {
			continue
		}

		snapshotDir := filepath.Join(snapshotsDir, entry.Name())
		meta, err := readSnapshotMetaForList(snapshotDir)
		if err != nil {
			return nil, err
		}

		commit := meta.CommitSHA
		if commit == "" {
			commit = entry.Name()
		}
		kind := meta.SnapshotKind
		if kind == "" {
			kind = snapshots.SnapshotKindNormal
		}

		rows = append(rows, listRow{
			Commit:                commit,
			Branch:                meta.Branch,
			CreatedAt:             meta.CreatedAt,
			Kind:                  kind,
			OrphanID:              "",
			TriggeringCheckoutSHA: "",
		})
	}

	if includeOrphans {
		orphanEntries, err := os.ReadDir(orphansDir)
		if os.IsNotExist(err) {
			orphanEntries = nil
			err = nil
		}
		if err != nil {
			return nil, fmt.Errorf("read orphans directory %q: %w", orphansDir, err)
		}

		for _, entry := range orphanEntries {
			if !entry.IsDir() {
				continue
			}
			if isInternalCollectionDir(entry.Name()) {
				continue
			}

			orphanDir := filepath.Join(orphansDir, entry.Name())
			meta, err := readSnapshotMetaForList(orphanDir)
			if err != nil {
				return nil, err
			}

			orphanID := meta.OrphanID
			if orphanID == "" {
				orphanID = entry.Name()
			}

			kind := meta.SnapshotKind
			if kind == "" {
				kind = snapshots.SnapshotKindOrphan
			}

			rows = append(rows, listRow{
				Commit:                "",
				Branch:                "",
				CreatedAt:             meta.CreatedAt,
				Kind:                  kind,
				OrphanID:              orphanID,
				TriggeringCheckoutSHA: meta.TriggeringCheckoutSHA,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		ti, okI := parseListTime(rows[i].CreatedAt)
		tj, okJ := parseListTime(rows[j].CreatedAt)
		if okI && okJ && !ti.Equal(tj) {
			return ti.After(tj)
		}
		if rows[i].CreatedAt != rows[j].CreatedAt {
			return rows[i].CreatedAt > rows[j].CreatedAt
		}
		if rows[i].Commit != rows[j].Commit {
			return rows[i].Commit < rows[j].Commit
		}
		return rows[i].OrphanID < rows[j].OrphanID
	})

	return rows, nil
}

func isInternalCollectionDir(name string) bool {
	// .tmp is publish staging for snapshots/orphans, not a user-facing entry.
	return name == ".tmp"
}

func readSnapshotMetaForList(snapshotDir string) (snapshots.Meta, error) {
	metaPath := filepath.Join(snapshotDir, "meta.json")
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return snapshots.Meta{}, fmt.Errorf("read snapshot meta at %q: %w", snapshotDir, err)
	}

	var meta snapshots.Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return snapshots.Meta{}, fmt.Errorf("parse snapshot meta at %q: %w", snapshotDir, err)
	}
	return meta, nil
}

func writeListRows(w io.Writer, rows []listRow, includeOrphans bool) error {
	if w == nil {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := "COMMIT\tBRANCH\tCREATED_AT\tKIND"
	if includeOrphans {
		header += "\tORPHAN_ID\tTRIGGERING_CHECKOUT_SHA"
	}
	if _, err := fmt.Fprintln(tw, header); err != nil {
		return fmt.Errorf("write list header: %w", err)
	}

	for _, row := range rows {
		line := fmt.Sprintf("%s\t%s\t%s\t%s", row.Commit, row.Branch, row.CreatedAt, row.Kind)
		if includeOrphans {
			line += fmt.Sprintf("\t%s\t%s", row.OrphanID, row.TriggeringCheckoutSHA)
		}
		if _, err := fmt.Fprintln(tw, line); err != nil {
			return fmt.Errorf("write list row for commit %q: %w", row.Commit, err)
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush list output: %w", err)
	}
	return nil
}

func parseListTime(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
