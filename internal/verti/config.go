package verti

import (
	"errors"

	"github.com/BurntSushi/toml"
)

type Config struct {
	RepoID    string
	Artifacts []string
}

type tomlConfig struct {
	Verti tomlVertiConfig `toml:"verti"`
}

type tomlVertiConfig struct {
	RepoID    string   `toml:"repo_id"`
	Artifacts []string `toml:"artifacts"`
}

func ReadConfig(path string) (Config, error) {
	var raw tomlConfig

	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return Config{}, err
	}
	if !meta.IsDefined("verti") {
		return Config{}, errors.New("missing [verti] config")
	}
	if raw.Verti.RepoID == "" {
		return Config{}, errors.New("empty repo_id")
	}

	cfg := Config{
		RepoID:    raw.Verti.RepoID,
		Artifacts: raw.Verti.Artifacts,
	}
	if cfg.Artifacts == nil {
		cfg.Artifacts = []string{}
	}

	return cfg, nil
}
