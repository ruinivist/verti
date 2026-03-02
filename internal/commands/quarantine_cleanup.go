package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const quarantineRetention = 7 * 24 * time.Hour

type quarantineMeta struct {
	CreatedAt  string `json:"created_at"`
	WorktreeID string `json:"worktree_id"`
}

func cleanupExpiredQuarantineSessions(storeRoot, repoID string, now time.Time) error {
	return cleanupQuarantineSessions(storeRoot, repoID, func(_ string, meta quarantineMeta) (bool, error) {
		if meta.CreatedAt == "" {
			return false, fmt.Errorf("missing created_at")
		}

		createdAt, err := time.Parse(time.RFC3339, meta.CreatedAt)
		if err != nil {
			return false, fmt.Errorf("parse created_at %q: %w", meta.CreatedAt, err)
		}

		cutoff := now.Add(-quarantineRetention)
		return !createdAt.After(cutoff), nil
	})
}

func cleanupWorktreeQuarantineSessions(storeRoot, repoID, worktreeID string) error {
	return cleanupQuarantineSessions(storeRoot, repoID, func(_ string, meta quarantineMeta) (bool, error) {
		return meta.WorktreeID == worktreeID, nil
	})
}

func cleanupQuarantineSessions(storeRoot, repoID string, shouldDelete func(sessionPath string, meta quarantineMeta) (bool, error)) error {
	quarantineRoot := filepath.Join(storeRoot, "repos", repoID, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read quarantine root %q: %w", quarantineRoot, err)
	}

	var errs error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionPath := filepath.Join(quarantineRoot, entry.Name())
		meta, err := readQuarantineMeta(filepath.Join(sessionPath, "meta.json"))
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("read quarantine metadata for %q: %w", sessionPath, err))
			continue
		}

		deleteSession, err := shouldDelete(sessionPath, meta)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("evaluate quarantine cleanup for %q: %w", sessionPath, err))
			continue
		}
		if !deleteSession {
			continue
		}

		if err := os.RemoveAll(sessionPath); err != nil {
			errs = errors.Join(errs, fmt.Errorf("remove quarantine session %q: %w", sessionPath, err))
		}
	}

	return errs
}

func readQuarantineMeta(path string) (quarantineMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return quarantineMeta{}, err
	}

	var meta quarantineMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return quarantineMeta{}, err
	}
	return meta, nil
}
