package restoremode

import (
	"fmt"
	"io"

	"verti/internal/config"
)

const (
	envRestoreMode = "VERTI_RESTORE_MODE"
	modeHint       = "prompt|force|skip"
)

// Resolve resolves restore mode from env override and config value.
// Order: env override -> config value -> default(prompt).
func Resolve(configMode string, env map[string]string, warnings io.Writer) (string, error) {
	if raw, ok := env[envRestoreMode]; ok && raw != "" {
		if err := validateMode(raw, "environment variable "+envRestoreMode); err != nil {
			return "", err
		}
		return raw, nil
	}

	mode := configMode
	if mode == "" {
		mode = config.RestoreModePrompt
	}
	if err := validateMode(mode, "config restore_mode"); err != nil {
		return "", err
	}

	_ = warnings
	return mode, nil
}

func validateMode(mode, source string) error {
	switch mode {
	case config.RestoreModePrompt, config.RestoreModeForce, config.RestoreModeSkip:
		return nil
	default:
		return fmt.Errorf("invalid %s=%q; expected one of %s", source, mode, modeHint)
	}
}
