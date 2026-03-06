package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHookDispatcherCapturesForeignHookToFirstBackupSlot(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, ReferenceTransactionHook)
	foreign := "#!/usr/bin/env bash\necho foreign\n"
	mustWriteExecutable(t, hookPath, foreign)

	_, err := InstallHookDispatcher(hookPath, ReferenceTransactionHook, "/abs/path/verti")
	if err != nil {
		t.Fatalf("InstallHookDispatcher() error = %v", err)
	}

	backupPath := hookPath + ".verti.orig-hooks.0"
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

func TestInstallHookDispatcherAlwaysAllocatesNewBackupSlotForDuplicateContent(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, ReferenceTransactionHook)
	foreign := "#!/usr/bin/env bash\necho foreign\n"

	mustWriteExecutable(t, hookPath+".verti.orig-hooks.0", foreign)
	mustWriteExecutable(t, hookPath, foreign)

	_, err := InstallHookDispatcher(hookPath, ReferenceTransactionHook, "/abs/path/verti")
	if err != nil {
		t.Fatalf("InstallHookDispatcher() error = %v", err)
	}

	if got := mustRead(t, hookPath+".verti.orig-hooks.1"); got != foreign {
		t.Fatalf("backup slot 1 mismatch:\n got %q\nwant %q", got, foreign)
	}

	dispatcher := mustRead(t, hookPath)
	if !strings.Contains(dispatcher, "LEGACY_HOOK=\""+hookPath+".verti.orig-hooks.1\"") {
		t.Fatalf("dispatcher does not point to latest backup slot:\n%s", dispatcher)
	}
}

func TestInstallHookDispatcherNoOpWhenDispatcherAlreadyInstalled(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, ReferenceTransactionHook)

	existing, err := DispatcherTemplate(ReferenceTransactionHook, "/abs/path/verti", "/abs/path/legacy")
	if err != nil {
		t.Fatalf("DispatcherTemplate() error = %v", err)
	}
	mustWriteExecutable(t, hookPath, existing)

	result, err := InstallHookDispatcher(hookPath, ReferenceTransactionHook, "/different/verti")
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

func TestRemoveVertiDispatcherRestoresLegacyHookWhenPresent(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, "post-commit")
	legacyPath := hookPath + ".verti.orig-hooks.0"
	legacy := "#!/usr/bin/env bash\necho legacy\n"
	mustWriteExecutable(t, legacyPath, legacy)

	dispatcher, err := DispatcherTemplate(ReferenceTransactionHook, "/abs/path/verti", legacyPath)
	if err != nil {
		t.Fatalf("DispatcherTemplate() error = %v", err)
	}
	mustWriteExecutable(t, hookPath, dispatcher)

	if err := RemoveVertiDispatcher(hookPath); err != nil {
		t.Fatalf("RemoveVertiDispatcher() error = %v", err)
	}

	if got := mustRead(t, hookPath); got != legacy {
		t.Fatalf("expected restored legacy hook\n got=%q\nwant=%q", got, legacy)
	}
}

func TestRemoveVertiDispatcherNoOpForForeignHook(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, "post-merge")
	foreign := "#!/usr/bin/env bash\necho foreign\n"
	mustWriteExecutable(t, hookPath, foreign)

	if err := RemoveVertiDispatcher(hookPath); err != nil {
		t.Fatalf("RemoveVertiDispatcher() error = %v", err)
	}

	if got := mustRead(t, hookPath); got != foreign {
		t.Fatalf("foreign hook should remain unchanged\n got=%q\nwant=%q", got, foreign)
	}
}

func TestParseBackupIndexAcceptsDottedNumericSuffixOnly(t *testing.T) {
	base := filepath.Join("/tmp", "reference-transaction"+backupSuffix)
	tests := []struct {
		name string
		path string
		want int
		ok   bool
	}{
		{name: "slot_zero", path: base + ".0", want: 0, ok: true},
		{name: "slot_twelve", path: base + ".12", want: 12, ok: true},
		{name: "plain_base_rejected", path: base, want: 0, ok: false},
		{name: "legacy_non_dotted_rejected", path: base + "1", want: 0, ok: false},
		{name: "empty_suffix_rejected", path: base + ".", want: 0, ok: false},
		{name: "negative_rejected", path: base + ".-1", want: 0, ok: false},
		{name: "nonnumeric_rejected", path: base + ".abc", want: 0, ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseBackupIndex(base, tc.path)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("parseBackupIndex(%q, %q) = (%d, %t), want (%d, %t)", base, tc.path, got, ok, tc.want, tc.ok)
			}
		})
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
