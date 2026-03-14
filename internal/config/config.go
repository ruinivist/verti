package config

import (
	"bytes"
	"errors"
	"os"

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

const managedHeader = "# Managed by Verti. Manual edits may be rewritten.\n# Comments and formatting in this file will be lost.\n\n"

func ReadConfig(path string) (Config, error) {
	var raw tomlConfig

	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return Config{}, err
	}
	if !meta.IsDefined("verti") {
		return Config{}, errors.New("missing [verti] config")
	}

	return normalizeConfig(Config{
		RepoID:    raw.Verti.RepoID,
		Artifacts: raw.Verti.Artifacts,
	})
}

func WriteConfig(path string, cfg Config) error {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString(managedHeader)
	err = toml.NewEncoder(&buf).Encode(tomlConfig{
		Verti: tomlVertiConfig{
			RepoID:    cfg.RepoID,
			Artifacts: cfg.Artifacts,
		},
	})
	if err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func normalizeConfig(cfg Config) (Config, error) {
	if cfg.RepoID == "" {
		return Config{}, errors.New("empty repo_id")
	}
	if cfg.Artifacts == nil {
		cfg.Artifacts = []string{}
	}
	return cfg, nil
}
