package verti

import (
	"verti/internal/snapshot"
)

func Sync() error {
	cfg, err := readCurrentConfig()
	if err != nil {
		return err
	}

	return snapshot.Sync(cfg)
}
