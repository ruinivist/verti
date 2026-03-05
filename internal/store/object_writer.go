package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var (
	writeAllData = func(f *os.File, b []byte) error {
		_, err := f.Write(b)
		return err
	}
)

// WriteObject stores data at <objectsDir>/<sha256>, using temp-file + fsync + atomic rename.
// If the object already exists, it is reused without rewriting.
func WriteObject(objectsDir string, data []byte) (string, error) {
	// fsync is for os level buffering, this is to flush writes basically
	// so they are durable once said to be done
	// the kernel syscall is fsync
	hash := sha256Hex(data)
	objectPath := filepath.Join(objectsDir, hash)

	if _, err := os.Stat(objectPath); err == nil {
		return hash, nil
	} else if !os.IsNotExist(err) {
		// if state failed for any reason that is non existent file, fail fast
		return "", fmt.Errorf("stat object %q: %w", objectPath, err)
	}

	if err := os.MkdirAll(objectsDir, 0o755); err != nil {
		return "", fmt.Errorf("create objects dir %q: %w", objectsDir, err)
	}

	tmpPath := filepath.Join(objectsDir, "."+hash+".tmp."+strconv.FormatInt(time.Now().UnixNano(), 10))
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("open temp object file %q: %w", tmpPath, err)
	}

	cleanupTemp := true
	defer func() {
		_ = file.Close()
		if cleanupTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := writeAllData(file, data); err != nil {
		return "", fmt.Errorf("write temp object file %q: %w", tmpPath, err)
	}

	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("fsync temp object file %q: %w", tmpPath, err)
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temp object file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, objectPath); err != nil {
		// Another writer may have created the same object first; treat as success.
		if _, statErr := os.Stat(objectPath); statErr == nil {
			return hash, nil
		}
		return "", fmt.Errorf("rename temp object %q to %q: %w", tmpPath, objectPath, err)
	}

	if err := syncDir(objectsDir); err != nil {
		return "", err
	}

	cleanupTemp = false
	return hash, nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open objects dir %q for fsync: %w", dir, err)
	}
	defer d.Close()

	if err := d.Sync(); err != nil {
		return fmt.Errorf("fsync objects dir %q: %w", dir, err)
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
