package main

import (
	"bytes"
	"strings"
	"testing"

	"verti/internal/cli"
)

func TestRunUnknownCommandShowsUsageAndNonZero(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"nope"}, &stdout, &stderr, cli.Handlers{})

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for unknown command")
	}
	if !strings.Contains(stderr.String(), "unknown command: nope") {
		t.Fatalf("expected unknown command error, got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage: verti <command> [args]") {
		t.Fatalf("expected usage text for unknown command, got: %q", stderr.String())
	}
}

func TestRunRestoreWithoutSHAShowsActionableUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"restore"}, &stdout, &stderr, cli.Handlers{})

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for restore without sha")
	}
	if !strings.Contains(stderr.String(), "restore requires a target SHA argument") {
		t.Fatalf("expected actionable restore usage error, got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage: verti <command> [args]") {
		t.Fatalf("expected usage text for restore error, got: %q", stderr.String())
	}
}

func TestRunDispatchesValidSubcommandsToHandlers(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		handler func(*bool) func([]string) error
	}{
		{
			name: "init",
			args: []string{"init"},
			handler: func(called *bool) func([]string) error {
				return func(_ []string) error {
					*called = true
					return nil
				}
			},
		},
		{
			name: "snapshot",
			args: []string{"snapshot"},
			handler: func(called *bool) func([]string) error {
				return func(_ []string) error {
					*called = true
					return nil
				}
			},
		},
		{
			name: "restore",
			args: []string{"restore", "abc123"},
			handler: func(called *bool) func([]string) error {
				return func(got []string) error {
					*called = true
					if len(got) != 1 || got[0] != "abc123" {
						t.Fatalf("unexpected restore args: %#v", got)
					}
					return nil
				}
			},
		},
		{
			name: "list",
			args: []string{"list"},
			handler: func(called *bool) func([]string) error {
				return func(_ []string) error {
					*called = true
					return nil
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			initCalled := false
			snapshotCalled := false
			restoreCalled := false
			listCalled := false

			handlers := cli.Handlers{
				Init:     func(_ []string) error { return nil },
				Snapshot: func(_ []string) error { return nil },
				Restore:  func(_ []string) error { return nil },
				List:     func(_ []string) error { return nil },
			}

			switch tc.name {
			case "init":
				handlers.Init = tc.handler(&initCalled)
			case "snapshot":
				handlers.Snapshot = tc.handler(&snapshotCalled)
			case "restore":
				handlers.Restore = tc.handler(&restoreCalled)
			case "list":
				handlers.List = tc.handler(&listCalled)
			}

			exitCode := run(tc.args, &stdout, &stderr, handlers)
			if exitCode != 0 {
				t.Fatalf("expected zero exit code, got %d (stderr=%q)", exitCode, stderr.String())
			}

			if tc.name == "init" && !initCalled {
				t.Fatalf("expected init handler to be called")
			}
			if tc.name == "snapshot" && !snapshotCalled {
				t.Fatalf("expected snapshot handler to be called")
			}
			if tc.name == "restore" && !restoreCalled {
				t.Fatalf("expected restore handler to be called")
			}
			if tc.name == "list" && !listCalled {
				t.Fatalf("expected list handler to be called")
			}
		})
	}
}
