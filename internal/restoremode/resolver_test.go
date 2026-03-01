package restoremode

import (
	"bytes"
	"strings"
	"testing"

	"verti/internal/config"
)

func TestResolveDefaultsToPrompt(t *testing.T) {
	var warnings bytes.Buffer

	mode, err := Resolve("", map[string]string{}, &warnings)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if mode != config.RestoreModePrompt {
		t.Fatalf("Resolve() = %q, want %q", mode, config.RestoreModePrompt)
	}
	if warnings.Len() != 0 {
		t.Fatalf("unexpected warnings for default mode: %q", warnings.String())
	}
}

func TestResolveEnvOverrideTakesPrecedence(t *testing.T) {
	var warnings bytes.Buffer

	mode, err := Resolve(config.RestoreModeSkip, map[string]string{
		"VERTI_RESTORE_MODE": config.RestoreModeForce,
	}, &warnings)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if mode != config.RestoreModeForce {
		t.Fatalf("Resolve() = %q, want %q", mode, config.RestoreModeForce)
	}
}

func TestResolveInvalidEnvModeReturnsActionableError(t *testing.T) {
	var warnings bytes.Buffer

	_, err := Resolve(config.RestoreModePrompt, map[string]string{
		"VERTI_RESTORE_MODE": "nope",
	}, &warnings)
	if err == nil {
		t.Fatalf("Resolve() expected error for invalid env mode")
	}
	if !strings.Contains(err.Error(), "VERTI_RESTORE_MODE") {
		t.Fatalf("expected env var name in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "prompt|force|skip") {
		t.Fatalf("expected actionable allowed-values hint, got %v", err)
	}
}

func TestResolveInvalidConfigModeReturnsActionableError(t *testing.T) {
	var warnings bytes.Buffer

	_, err := Resolve("bad", map[string]string{}, &warnings)
	if err == nil {
		t.Fatalf("Resolve() expected error for invalid config mode")
	}
	if !strings.Contains(err.Error(), "restore_mode") {
		t.Fatalf("expected restore_mode in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "prompt|force|skip") {
		t.Fatalf("expected actionable allowed-values hint, got %v", err)
	}
}
