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
	Commit    string
	Branch    string
	CreatedAt string
	Kind      string
}

// RunList prints snapshots for the current worktree.
func RunList(workingDir string, args []string) error {
	return runList(workingDir, args, os.Stdout)
}

func runList(workingDir string, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return &cli.UsageError{Message: "list does not accept positional arguments"}
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

	snapshotsDir := filepath.Join(storeRoot, "repos", cfg.RepoID, "worktrees", worktreeID.WorktreeID, "snapshots")
	rows, err := loadListRows(snapshotsDir)
	if err != nil {
		return err
	}

	return writeListRows(stdout, rows)
}

func loadListRows(snapshotsDir string) ([]listRow, error) {
	entries, err := os.ReadDir(snapshotsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshots directory %q: %w", snapshotsDir, err)
	}

	rows := make([]listRow, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == ".tmp" {
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
			Commit:    commit,
			Branch:    meta.Branch,
			CreatedAt: meta.CreatedAt,
			Kind:      kind,
		})
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
		return rows[i].Commit < rows[j].Commit
	})

	return rows, nil
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

func writeListRows(w io.Writer, rows []listRow) error {
	if w == nil {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "COMMIT\tBRANCH\tCREATED_AT\tKIND"); err != nil {
		return fmt.Errorf("write list header: %w", err)
	}

	for _, row := range rows {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", row.Commit, row.Branch, row.CreatedAt, row.Kind); err != nil {
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
