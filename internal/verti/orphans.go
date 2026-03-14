package verti

import (
	"errors"
	"fmt"
	"time"

	verticonfig "verti/internal/config"
	"verti/internal/gitrepo"
	"verti/internal/output"
	"verti/internal/snapshot"
)

func Orphans() error {
	cfg, err := readCurrentConfig()
	if err != nil {
		return err
	}

	items, err := snapshot.ListOrphans(cfg)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		output.Println("no orphan snapshots")
		return nil
	}

	now := time.Now()
	for i, item := range items {
		output.Printf(
			"%d. %s (%s) - %s\n",
			i+1,
			formatRelativeTime(now, item.CreatedAt),
			formatLocalTime(item.CreatedAt),
			artifactCountLabel(item.ArtifactCount),
		)
	}

	return nil
}

func RestoreOrphan(index int) error {
	cfg, err := readCurrentConfig()
	if err != nil {
		return err
	}

	item, err := snapshot.RestoreOrphan(cfg, index)
	if err != nil {
		return err
	}

	output.Printf("Restored orphan #%d (%s)\n", index, item.ID)
	return nil
}

func readCurrentConfig() (verticonfig.Config, error) {
	if err := gitrepo.EnsureGitDir(); err != nil {
		return verticonfig.Config{}, errors.New("not a git repository")
	}

	cfg, err := verticonfig.ReadConfig(configPath)
	if err != nil {
		return verticonfig.Config{}, fmt.Errorf("failed to read config: %v", err)
	}

	return cfg, nil
}

func formatRelativeTime(now, at time.Time) string {
	if now.Before(at) {
		now = at
	}

	delta := now.Sub(at)
	switch {
	case delta < time.Minute:
		return pluralizeDuration("second", int(delta/time.Second))
	case delta < time.Hour:
		return pluralizeDuration("minute", int(delta/time.Minute))
	case delta < 24*time.Hour:
		return pluralizeDuration("hour", int(delta/time.Hour))
	default:
		return pluralizeDuration("day", int(delta/(24*time.Hour)))
	}
}

func pluralizeDuration(unit string, count int) string {
	if count < 1 {
		count = 1
	}
	if count == 1 {
		return fmt.Sprintf("1 %s ago", unit)
	}
	return fmt.Sprintf("%d %ss ago", count, unit)
}

func formatLocalTime(at time.Time) string {
	return at.In(time.Local).Format("2006-01-02 15:04:05 -0700")
}

func artifactCountLabel(count int) string {
	if count == 1 {
		return "1 artifact"
	}
	return fmt.Sprintf("%d artifacts", count)
}
