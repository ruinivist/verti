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
	hookPath := filepath.Join(hookDir, PostCommitHook)
	foreign := "#!/usr/bin/env bash\necho foreign\n"

	mustWriteExecutable(t, hookPath+".verti.orig-hooks.0", foreign)
	mustWriteExecutable(t, hookPath, foreign)

	_, err := InstallHookDispatcher(hookPath, PostCommitHook, "/abs/path/verti")
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

	if got := mustRead(t, hookPath+".verti.orig-hooks.0"); got != foreignA {
		t.Fatalf("backup slot 0 mismatch:\n got %q\nwant %q", got, foreignA)
	}
	if got := mustRead(t, hookPath+".verti.orig-hooks.1"); got != foreignB {
		t.Fatalf("backup slot 1 mismatch:\n got %q\nwant %q", got, foreignB)
	}

	dispatcher := mustRead(t, hookPath)
	if !strings.Contains(dispatcher, "LEGACY_HOOK=\""+hookPath+".verti.orig-hooks.1\"") {
		t.Fatalf("dispatcher does not point to latest backup slot:\n%s", dispatcher)
	}
	if strings.Contains(dispatcher, "LEGACY_HOOK=\""+hookPath+".verti.orig-hooks.0\"") {
		t.Fatalf("dispatcher should not point to old backup slot:\n%s", dispatcher)
	}
}

func TestInstallHookDispatcherAppendsAfterHighestExistingSlot(t *testing.T) {
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, PostMergeHook)

	mustWriteExecutable(t, hookPath+".verti.orig-hooks.0", "#!/usr/bin/env bash\necho zero\n")
	mustWriteExecutable(t, hookPath+".verti.orig-hooks.2", "#!/usr/bin/env bash\necho two\n")

	foreign := "#!/usr/bin/env bash\necho latest\n"
	mustWriteExecutable(t, hookPath, foreign)

	_, err := InstallHookDispatcher(hookPath, PostMergeHook, "/abs/path/verti")
	if err != nil {
		t.Fatalf("InstallHookDispatcher() error = %v", err)
	}

	if got := mustRead(t, hookPath+".verti.orig-hooks.3"); got != foreign {
		t.Fatalf("backup slot 3 mismatch:\n got %q\nwant %q", got, foreign)
	}

	dispatcher := mustRead(t, hookPath)
	if !strings.Contains(dispatcher, "LEGACY_HOOK=\""+hookPath+".verti.orig-hooks.3\"") {
		t.Fatalf("dispatcher should point to next slot after current max:\n%s", dispatcher)
	}
}

func TestParseBackupIndexAcceptsDottedNumericSuffixOnly(t *testing.T) {
	base := filepath.Join("/tmp", "post-commit"+backupSuffix)
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
