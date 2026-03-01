package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestLoadReturnsDefaultsWhenConfigFileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verti.toml")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := Default()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestSaveAndLoadRoundTripPreservesFieldsIncludingRepoID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verti.toml")

	want := Config{
		RepoID:        "123e4567-e89b-12d3-a456-426614174000",
		Enabled:       false,
		Artifacts:     []string{"md", "progress.md", "notes"},
		StoreRoot:     "~/.verti-test",
		RestoreMode:   RestoreModeForce,
		MaxFileSizeMB: 64,
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\n got  %#v\n want %#v", got, want)
	}
}

func TestLoadRejectsInvalidRestoreMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verti.toml")

	const badConfig = `
restore_mode = "nope"
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(badConfig)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load() expected error for invalid restore_mode")
	}
	if !strings.Contains(err.Error(), "restore_mode") {
		t.Fatalf("expected restore_mode in error, got %v", err)
	}
}

func TestLoadRejectsOutOfBoundsMaxFileSizeMB(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{name: "too_small", value: 0},
		{name: "too_large", value: MaxMaxFileSizeMB + 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "verti.toml")
			content := []byte("max_file_size_mb = " + strconv.Itoa(tc.value) + "\n")
			if err := os.WriteFile(path, content, 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := Load(path)
			if err == nil {
				t.Fatalf("Load() expected bounds error for max_file_size_mb=%d", tc.value)
			}
			if !strings.Contains(err.Error(), "max_file_size_mb") {
				t.Fatalf("expected max_file_size_mb in error, got %v", err)
			}
		})
	}
}
