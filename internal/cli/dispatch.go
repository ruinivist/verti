package cli

import (
	"fmt"
	"io"
)

// Handler executes a subcommand with remaining CLI args.
type Handler func(args []string) error

// Handlers groups handlers for each supported subcommand.
type Handlers struct {
	Init     Handler
	Snapshot Handler
	Restore  Handler
	List     Handler
}

// UsageError indicates incorrect CLI usage.
type UsageError struct {
	Message string
}

func (e *UsageError) Error() string {
	return e.Message
}

const usageText = `Usage: verti <command> [args]
Commands:
  init
  snapshot
  restore <sha>
  list`

// WriteUsage prints usage text.
func WriteUsage(w io.Writer) {
	fmt.Fprintln(w, usageText)
}

// NotImplementedHandlers returns stub handlers for bootstrap phase.
func NotImplementedHandlers() Handlers {
	return Handlers{
		Init:     notImplemented("init"),
		Snapshot: notImplemented("snapshot"),
		Restore:  notImplemented("restore"),
		List:     notImplemented("list"),
	}
}

func notImplemented(command string) Handler {
	return func(_ []string) error {
		return fmt.Errorf("%s: not implemented", command)
	}
}

// Dispatch routes args to a subcommand handler.
func Dispatch(args []string, handlers Handlers) error {
	if len(args) == 0 {
		return &UsageError{Message: "missing command"}
	}

	command := args[0]
	rest := args[1:]

	switch command {
	case "init":
		return callHandler("init", handlers.Init, rest)
	case "snapshot":
		return callHandler("snapshot", handlers.Snapshot, rest)
	case "restore":
		if len(rest) == 0 {
			return &UsageError{Message: "restore requires a target SHA argument"}
		}
		return callHandler("restore", handlers.Restore, rest)
	case "list":
		return callHandler("list", handlers.List, rest)
	default:
		return &UsageError{Message: fmt.Sprintf("unknown command: %s", command)}
	}
}

func callHandler(command string, handler Handler, args []string) error {
	if handler == nil {
		return fmt.Errorf("%s handler is not configured", command)
	}
	return handler(args)
}
