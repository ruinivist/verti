package snapshot

import (
	"fmt"
	"os"
	"time"

	verticonfig "verti/internal/config"
)

type OrphanItem struct {
	ID            string
	CreatedAt     time.Time
	ArtifactCount int
}

func ListOrphans(cfg verticonfig.Config) ([]OrphanItem, error) {
	store, err := prepareStore(cfg.RepoID)
	if err != nil {
		return nil, err
	}

	manifests, err := loadOrphanManifests(store)
	if err != nil {
		return nil, err
	}
	if len(manifests) > orphanLimit {
		manifests = manifests[:orphanLimit]
	}

	items := make([]OrphanItem, 0, len(manifests))
	for _, item := range manifests {
		items = append(items, OrphanItem{
			ID:            item.id,
			CreatedAt:     item.createdAt,
			ArtifactCount: len(item.manifest.Artifacts),
		})
	}

	return items, nil
}

func RestoreOrphan(cfg verticonfig.Config, index int) (OrphanItem, error) {
	store, err := prepareStore(cfg.RepoID)
	if err != nil {
		return OrphanItem{}, err
	}

	manifests, err := loadOrphanManifests(store)
	if err != nil {
		return OrphanItem{}, err
	}
	if index < 1 || index > len(manifests) {
		return OrphanItem{}, fmt.Errorf("orphan number out of range: %d", index)
	}

	selected := manifests[index-1]
	if err := restoreArtifacts(store, selected.manifest); err != nil {
		return OrphanItem{}, err
	}

	return OrphanItem{
		ID:            selected.id,
		CreatedAt:     selected.createdAt,
		ArtifactCount: len(selected.manifest.Artifacts),
	}, nil
}

func prepareStore(repoID string) (store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return store{}, fmt.Errorf("failed to resolve home: %v", err)
	}

	repoStore := newStore(home, repoID)
	if err := repoStore.ensureDirs(); err != nil {
		return store{}, err
	}
	if err := cleanupOrphans(repoStore); err != nil {
		return store{}, err
	}

	return repoStore, nil
}
