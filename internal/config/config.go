package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	RestoreModePrompt = "prompt"
	RestoreModeForce  = "force"
	RestoreModeSkip   = "skip"

	DefaultStoreRoot     = "~/.verti"
	DefaultMaxFileSizeMB = 32
	MinMaxFileSizeMB     = 1
	MaxMaxFileSizeMB     = 1024
)

// Config models <common_git_dir>/verti.toml.
type Config struct {
	RepoID        string   `toml:"repo_id"`
	Enabled       bool     `toml:"enabled"`
	Artifacts     []string `toml:"artifacts"`
	StoreRoot     string   `toml:"store_root"`
	RestoreMode   string   `toml:"restore_mode"`
	MaxFileSizeMB int      `toml:"max_file_size_mb"`
}

// Default returns the MVP default config values.
func Default() Config {
	return Config{
		RepoID:        "",
		Enabled:       true,
		Artifacts:     []string{"md", "progress.md"},
		StoreRoot:     DefaultStoreRoot,
		RestoreMode:   RestoreModePrompt,
		MaxFileSizeMB: DefaultMaxFileSizeMB,
	}
}

// Load reads config from path.
// If path does not exist, it returns defaults.
func Load(path string) (Config, error) {
	cfg := Default()

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("stat config %q: %w", path, err)
	}

	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		return Config{}, fmt.Errorf("config %q contains unknown field(s): %s", path, strings.Join(keys, ", "))
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config %q: %w", path, err)
	}

	return cfg, nil
}

// Save validates and writes config to path as TOML.
func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir for %q: %w", path, err)
	}

	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open temp config %q: %w", tmpPath, err)
	}

	encodeErr := toml.NewEncoder(file).Encode(cfg)
	closeErr := file.Close()
	if encodeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("encode config %q: %w", path, encodeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp config %q: %w", tmpPath, closeErr)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp config into %q: %w", path, err)
	}

	return nil
}

// Validate checks config values for supported bounds.
func (c Config) Validate() error {
	switch c.RestoreMode {
	case RestoreModePrompt, RestoreModeForce, RestoreModeSkip:
	default:
		return fmt.Errorf("restore_mode must be one of %q, %q, %q (got %q)", RestoreModePrompt, RestoreModeForce, RestoreModeSkip, c.RestoreMode)
	}

	if c.MaxFileSizeMB < MinMaxFileSizeMB || c.MaxFileSizeMB > MaxMaxFileSizeMB {
		return fmt.Errorf("max_file_size_mb must be between %d and %d (got %d)", MinMaxFileSizeMB, MaxMaxFileSizeMB, c.MaxFileSizeMB)
	}

	return nil
}
