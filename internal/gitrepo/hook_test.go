package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPostCheckoutHookTriggersOnlyForForcedSameCommitCheckout(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sync.log")
	exePath := filepath.Join(dir, "verti")
	hookPath := filepath.Join(dir, "post-checkout")

	if err := os.WriteFile(exePath, []byte("#!/bin/sh\necho \"$@\" >> "+strconv.Quote(logPath)+"\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	if err := WritePostCheckoutHook(hookPath, exePath); err != nil {
		t.Fatalf("WritePostCheckoutHook() error = %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		wantLog string
	}{
		{
			name:    "forced same commit checkout",
			args:    []string{"abc", "abc", "1"},
			wantLog: "sync\n",
		},
		{
			name:    "branch switch to different commit",
			args:    []string{"abc", "def", "1"},
			wantLog: "",
		},
		{
			name:    "path checkout",
			args:    []string{"abc", "abc", "0"},
			wantLog: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.RemoveAll(logPath); err != nil {
				t.Fatalf("clear log: %v", err)
			}

			cmd := exec.Command("sh", append([]string{hookPath}, tt.args...)...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run hook: %v\n%s", err, string(out))
			}

			logData, err := os.ReadFile(logPath)
			if err != nil {
				if !os.IsNotExist(err) {
					t.Fatalf("read log: %v", err)
				}
				if tt.wantLog != "" {
					t.Fatalf("log missing, want %q", tt.wantLog)
				}
				return
			}
			if string(logData) != tt.wantLog {
				t.Fatalf("log = %q, want %q", string(logData), tt.wantLog)
			}
		})
	}
}
