package hooks

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	dispatcherMarker = "# verti-hooks"
	backupSuffix     = ".verti.orig-hooks"
)

// InstallResult describes what happened during dispatcher installation.
type InstallResult struct {
	NoOp           bool
	LegacyHookPath string
}

// InstallHookDispatcher installs a Verti dispatcher at hookPath and preserves any existing hook in backup slots.
func InstallHookDispatcher(hookPath, hookName, vertiBinPath string) (InstallResult, error) {
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create hook dir for %q: %w", hookPath, err)
	}

	currentHook, err := readFileWithInfo(hookPath)
	if err != nil {
		return InstallResult{}, err
	}

	if currentHook.exists && bytes.Contains(currentHook.data, []byte(dispatcherMarker)) {
		return InstallResult{NoOp: true}, nil
	}

	// the default path is .0 based even if nothing exists
	// the hooks written has a guard for it
	// if it's written then we get a new path
	legacyHookPath := backupSlotPath(hookPath, 0)
	if currentHook.exists {
		legacyHookPath, err = ensureBackupSlot(hookPath, currentHook.data, currentHook.info.Mode().Perm())
		if err != nil {
			return InstallResult{}, err
		}
	}

	dispatcher, err := DispatcherTemplate(hookName, vertiBinPath, legacyHookPath)
	if err != nil {
		return InstallResult{}, fmt.Errorf("build dispatcher for %q: %w", hookName, err)
	}

	if err := writeFileAtomically(hookPath, []byte(dispatcher), 0o755); err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		NoOp:           false,
		LegacyHookPath: legacyHookPath,
	}, nil
}

// RemoveVertiDispatcher removes a Verti-managed dispatcher hook, restoring
// the captured legacy hook if the dispatcher points to one that still exists.
func RemoveVertiDispatcher(hookPath string) error {
	currentHook, err := readFileWithInfo(hookPath)
	if err != nil {
		return err
	}
	if !currentHook.exists {
		return nil
	}
	if !bytes.Contains(currentHook.data, []byte(dispatcherMarker)) {
		return nil
	}

	legacyPath := legacyHookPathFromDispatcher(currentHook.data)
	if legacyPath != "" {
		legacyHook, err := readFileWithInfo(legacyPath)
		if err == nil && legacyHook.exists {
			mode := modeOrDefault(legacyHook.info.Mode().Perm(), 0o755)
			if err := writeFileAtomically(hookPath, legacyHook.data, mode); err != nil {
				return err
			}
			return nil
		}
	}

	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove legacy dispatcher %q: %w", hookPath, err)
	}
	return nil
}

// ensureBackupSlot writes foreignContent to the next backup slot and returns that slot path.
func ensureBackupSlot(hookPath string, foreignContent []byte, mode os.FileMode) (string, error) {
	next, err := nextBackupSlot(hookPath)
	if err != nil {
		return "", err
	}
	slotPath := backupSlotPath(hookPath, next)
	if err := writeFileAtomically(slotPath, foreignContent, modeOrDefault(mode, 0o755)); err != nil {
		return "", err
	}
	return slotPath, nil
}

// parseBackupIndex parses a dotted backup slot suffix and returns its numeric index.
func parseBackupIndex(base, path string) (int, bool) {
	prefix := base + "."
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	suffix := strings.TrimPrefix(path, prefix)
	if suffix == "" {
		return 0, false
	}
	idx, err := strconv.Atoi(suffix)
	if err != nil || idx < 0 {
		return 0, false
	}
	return idx, true
}

// nextBackupSlot scans dotted backup files and returns the next monotonic slot index after the current maximum.
func nextBackupSlot(hookPath string) (int, error) {
	base := hookPath + backupSuffix
	pattern := base + ".*"

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0, fmt.Errorf("scan backup slots for %q: %w", hookPath, err)
	}

	next := 0
	for _, match := range matches {
		idx, ok := parseBackupIndex(base, match)
		if !ok {
			continue
		}
		if idx >= next {
			next = idx + 1
		}
	}

	return next, nil
}

// backupSlotPath builds the full backup file path for a given hook path and slot index.
func backupSlotPath(hookPath string, slot int) string {
	base := hookPath + backupSuffix
	return base + "." + strconv.Itoa(slot)
}

// modeOrDefault returns mode unless it is zero, in which case it returns fallback.
func modeOrDefault(mode, fallback os.FileMode) os.FileMode {
	if mode == 0 {
		return fallback
	}
	return mode
}

func legacyHookPathFromDispatcher(data []byte) string {
	const marker = "LEGACY_HOOK=\""
	idx := bytes.Index(data, []byte(marker))
	if idx < 0 {
		return ""
	}
	start := idx + len(marker)
	if start >= len(data) {
		return ""
	}

	end := bytes.IndexByte(data[start:], '"')
	if end < 0 {
		return ""
	}

	return string(data[start : start+end])
}

type fileReadResult struct {
	data   []byte
	info   os.FileInfo
	exists bool
}

// readFileWithInfo reads file bytes and stat info, reporting exists=false when the path is missing.
func readFileWithInfo(path string) (fileReadResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileReadResult{}, nil
		}
		return fileReadResult{}, fmt.Errorf("stat %q: %w", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fileReadResult{}, fmt.Errorf("read %q: %w", path, err)
	}

	return fileReadResult{
		data:   data,
		info:   info,
		exists: true,
	}, nil
}

// writeFileAtomically writes data via a temp file and renames it into place with the requested mode.
func writeFileAtomically(path string, data []byte, mode os.FileMode) error {
	tmpPath := path + ".tmp"

	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("open temp file %q: %w", tmpPath, err)
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file %q: %w", tmpPath, err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file %q: %w", tmpPath, err)
	}

	if err := os.Chmod(tmpPath, mode); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file %q to %q: %w", tmpPath, path, err)
	}

	return nil
}
