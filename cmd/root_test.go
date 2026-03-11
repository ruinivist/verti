package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantErr  string
		wantCode int
	}{
		{
			name:     "no args",
			wantOut:  "unknown command\n",
			wantCode: 1,
		},
		{
			name:     "init",
			args:     []string{"init"},
			wantOut:  "init\n",
			wantCode: 0,
		},
		{
			name:     "sync",
			args:     []string{"sync"},
			wantOut:  "sync\n",
			wantCode: 0,
		},
		{
			name:     "unknown command",
			args:     []string{"wat"},
			wantOut:  "unknown command: wat\n",
			wantCode: 1,
		},
		{
			name:     "sync extra",
			args:     []string{"sync", "extra", "bits"},
			wantOut:  "unknown sync option: extra bits\n",
			wantCode: 1,
		},
		{
			name:     "init extra",
			args:     []string{"init", "extra"},
			wantOut:  "unknown init option: extra\n",
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := captureOutput(t, func() {
				if got := Run(tt.args); got != tt.wantCode {
					t.Fatalf("Run() code = %d, want %d", got, tt.wantCode)
				}
			})

			if stdout != tt.wantOut {
				t.Fatalf("stdout = %q, want %q", stdout, tt.wantOut)
			}
			if stderr != tt.wantErr {
				t.Fatalf("stderr = %q, want %q", stderr, tt.wantErr)
			}
		})
	}
}

func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdout: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stderr: %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	fn()

	if err := stdoutW.Close(); err != nil {
		t.Fatalf("stdout close: %v", err)
	}
	if err := stderrW.Close(); err != nil {
		t.Fatalf("stderr close: %v", err)
	}

	var stdoutBuf bytes.Buffer
	if _, err := io.Copy(&stdoutBuf, stdoutR); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	var stderrBuf bytes.Buffer
	if _, err := io.Copy(&stderrBuf, stderrR); err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	if err := stdoutR.Close(); err != nil {
		t.Fatalf("stdout reader close: %v", err)
	}
	if err := stderrR.Close(); err != nil {
		t.Fatalf("stderr reader close: %v", err)
	}

	return stdoutBuf.String(), stderrBuf.String()
}
