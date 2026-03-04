package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHookDispatcherCapturesForeignHookToFirstBackupSlot(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, PostCommitHook)
	foreign := "#!/usr/bin/env bash\necho foreign\n"
	mustWriteExecutable(t, hookPath, foreign)

	_, err := InstallHookDispatcher(hookPath, PostCommitHook, "/abs/path/verti")
	if err != nil {
		t.Fatalf("InstallHookDispatcher() error = %v", err)
	}

	backupPath := hookPath + ".verti.backup"
	gotBackup := mustRead(t, backupPath)
	if gotBackup != foreign {
		t.Fatalf("backup content mismatch:\n got %q\nwant %q", gotBackup, foreign)
	}

	dispatcher := mustRead(t, hookPath)
	if !strings.Contains(dispatcher, "# verti-hooks") {
		t.Fatalf("dispatcher missing marker:\n%s", dispatcher)
	}
	if !strings.Contains(dispatcher, "LEGACY_HOOK=\""+backupPath+"\"") {
		t.Fatalf("dispatcher missing backup slot path %q:\n%s", backupPath, dispatcher)
	}
}

func TestInstallHookDispatcherReusesExistingBackupSlotForDuplicateContent(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, PostCommitHook)
	foreign := "#!/usr/bin/env bash\necho foreign\n"

	mustWriteExecutable(t, hookPath+".verti.backup", foreign)
	mustWriteExecutable(t, hookPath, foreign)

	_, err := InstallHookDispatcher(hookPath, PostCommitHook, "/abs/path/verti")
	if err != nil {
		t.Fatalf("InstallHookDispatcher() error = %v", err)
	}

	if _, err := os.Stat(hookPath + ".verti.backup1"); err == nil {
		t.Fatalf("expected no new backup1 slot when duplicate content exists")
	}

	dispatcher := mustRead(t, hookPath)
	if !strings.Contains(dispatcher, "LEGACY_HOOK=\""+hookPath+".verti.backup\"") {
		t.Fatalf("dispatcher did not reuse existing backup slot:\n%s", dispatcher)
	}
}

func TestInstallHookDispatcherNoOpWhenDispatcherAlreadyInstalled(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, PostCheckoutHook)

	existing, err := DispatcherTemplate(PostCheckoutHook, "/abs/path/verti", "/abs/path/legacy")
	if err != nil {
		t.Fatalf("DispatcherTemplate() error = %v", err)
	}
	mustWriteExecutable(t, hookPath, existing)

	result, err := InstallHookDispatcher(hookPath, PostCheckoutHook, "/different/verti")
	if err != nil {
		t.Fatalf("InstallHookDispatcher() error = %v", err)
	}
	if !result.NoOp {
		t.Fatalf("expected no-op result when dispatcher marker already exists")
	}

	got := mustRead(t, hookPath)
	if got != existing {
		t.Fatalf("expected hook content unchanged on no-op:\n got %q\nwant %q", got, existing)
	}
}

func TestInstallHookDispatcherCapturesOverwriteToNextSlotAndPointsDispatcherToLatest(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, PostMergeHook)

	foreignA := "#!/usr/bin/env bash\necho first\n"
	mustWriteExecutable(t, hookPath, foreignA)
	_, err := InstallHookDispatcher(hookPath, PostMergeHook, "/abs/path/verti")
	if err != nil {
		t.Fatalf("InstallHookDispatcher(first) error = %v", err)
	}

	foreignB := "#!/usr/bin/env bash\necho second\n"
	mustWriteExecutable(t, hookPath, foreignB) // simulate third-party overwrite after init

	_, err = InstallHookDispatcher(hookPath, PostMergeHook, "/abs/path/verti")
	if err != nil {
		t.Fatalf("InstallHookDispatcher(second) error = %v", err)
	}

	if got := mustRead(t, hookPath+".verti.backup"); got != foreignA {
		t.Fatalf("backup slot 0 mismatch:\n got %q\nwant %q", got, foreignA)
	}
	if got := mustRead(t, hookPath+".verti.backup1"); got != foreignB {
		t.Fatalf("backup slot 1 mismatch:\n got %q\nwant %q", got, foreignB)
	}

	dispatcher := mustRead(t, hookPath)
	if !strings.Contains(dispatcher, "LEGACY_HOOK=\""+hookPath+".verti.backup1\"") {
		t.Fatalf("dispatcher does not point to latest backup slot:\n%s", dispatcher)
	}
	if strings.Contains(dispatcher, "LEGACY_HOOK=\""+hookPath+".verti.backup\"") {
		t.Fatalf("dispatcher should not point to old backup slot:\n%s", dispatcher)
	}
}

func mustWriteExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
