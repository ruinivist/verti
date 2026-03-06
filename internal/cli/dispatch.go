package cli

import (
	"fmt"
	"io"
)

// Handler executes a subcommand with remaining CLI args.
type Handler func(args []string) error

// Handlers groups handlers for each supported subcommand.
type Handlers struct {
	Init Handler
	Sync Handler
	List Handler
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
  sync [--debounced]
  list`

// WriteUsage prints usage text.
func WriteUsage(w io.Writer) {
	fmt.Fprintln(w, usageText)
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
	case "sync":
		return callHandler("sync", handlers.Sync, rest)
	case "list":
		return callHandler("list", handlers.List, rest)
	default:
		return &UsageError{Message: fmt.Sprintf("unknown command: %s", command)}
	}
}

// this is a just a nil check wrapper to avoid ugly panics, in case
// handler is used with no corresponding function set
func callHandler(command string, handler Handler, args []string) error {
	if handler == nil {
		return fmt.Errorf("%s handler is not configured", command)
	}
	return handler(args)
}
