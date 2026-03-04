package hooks

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	dispatcherMarker = "# verti-hooks"
	backupSuffix     = ".verti.backup"
)

// InstallResult describes what happened during dispatcher installation.
type InstallResult struct {
	NoOp           bool
	LegacyHookPath string
}

// InstallHookDispatcher installs a Verti disiatiher at hookPath with backup of existing
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

	legacyHookPath := hookPath + backupSuffix
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

func ensureBackupSlot(hookPath string, foreignContent []byte, mode os.FileMode) (string, error) {
	slots, err := existingBackupSlots(hookPath)
	if err != nil {
		return "", err
	}

	for _, idx := range sortedKeys(slots) {
		slotPath := slots[idx]
		slot, err := readFileWithInfo(slotPath)
		if err != nil {
			return "", err
		}
		if slot.exists && bytes.Equal(slot.data, foreignContent) {
			return slotPath, nil
		}
	}

	next := firstMissingSlot(slots)
	slotPath := backupSlotPath(hookPath, next)
	if err := writeFileAtomically(slotPath, foreignContent, modeOrDefault(mode, 0o755)); err != nil {
		return "", err
	}
	return slotPath, nil
}

func existingBackupSlots(hookPath string) (map[int]string, error) {
	base := hookPath + backupSuffix
	pattern := base + "*"

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("scan backup slots for %q: %w", hookPath, err)
	}

	slots := make(map[int]string, len(matches))
	for _, match := range matches {
		idx, ok := parseBackupIndex(base, match)
		if !ok {
			continue
		}
		slots[idx] = match
	}

	return slots, nil
}

func parseBackupIndex(base, path string) (int, bool) {
	if path == base {
		return 0, true
	}
	if !strings.HasPrefix(path, base) {
		return 0, false
	}
	suffix := strings.TrimPrefix(path, base)
	if suffix == "" {
		return 0, true
	}
	idx, err := strconv.Atoi(suffix)
	if err != nil || idx < 1 {
		return 0, false
	}
	return idx, true
}

func firstMissingSlot(slots map[int]string) int {
	for i := 0; ; i++ {
		if _, ok := slots[i]; !ok {
			return i
		}
	}
}

func backupSlotPath(hookPath string, slot int) string {
	base := hookPath + backupSuffix
	if slot == 0 {
		return base
	}
	return base + strconv.Itoa(slot)
}

func sortedKeys(m map[int]string) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func modeOrDefault(mode, fallback os.FileMode) os.FileMode {
	if mode == 0 {
		return fallback
	}
	return mode
}

type fileReadResult struct {
	data   []byte
	info   os.FileInfo
	exists bool
}

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
