package main

import (
	"bytes"
	"strings"
	"testing"

	"verti/internal/cli"
	"verti/internal/commands"
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

func TestRunSyncWithUnexpectedArgsShowsActionableUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	handlers := cli.Handlers{
		Sync: newHandler(commands.RunSync),
	}
	exitCode := run([]string{"sync", "unexpected"}, &stdout, &stderr, handlers)

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for sync with unexpected args")
	}
	if !strings.Contains(stderr.String(), "sync accepts no positional args") {
		t.Fatalf("expected actionable sync usage error, got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage: verti <command> [args]") {
		t.Fatalf("expected usage text for sync error, got: %q", stderr.String())
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
			name: "sync",
			args: []string{"sync"},
			handler: func(called *bool) func([]string) error {
				return func(_ []string) error {
					*called = true
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
			syncCalled := false
			listCalled := false

			handlers := cli.Handlers{
				Init: func(_ []string) error { return nil },
				Sync: func(_ []string) error { return nil },
				List: func(_ []string) error { return nil },
			}

			switch tc.name {
			case "init":
				handlers.Init = tc.handler(&initCalled)
			case "sync":
				handlers.Sync = tc.handler(&syncCalled)
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
			if tc.name == "sync" && !syncCalled {
				t.Fatalf("expected sync handler to be called")
			}
			if tc.name == "list" && !listCalled {
				t.Fatalf("expected list handler to be called")
			}
		})
	}
}
