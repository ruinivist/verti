package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"verti/internal/cli"
	"verti/internal/commands"
)

func main() {
	handlers := cli.NotImplementedHandlers()
	handlers.Init = func(_ []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
		return commands.RunInit(wd)
	}
	handlers.Snapshot = func(_ []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
		return commands.RunSnapshot(wd)
	}
	handlers.Restore = func(args []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
		return commands.RunRestore(wd, args)
	}

	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, handlers))
}

func run(args []string, stdout, stderr io.Writer, handlers cli.Handlers) int {
	_ = stdout

	err := cli.Dispatch(args, handlers)
	if err == nil {
		return 0
	}

	fmt.Fprintf(stderr, "verti: %v\n", err)

	var usageErr *cli.UsageError
	if errors.As(err, &usageErr) {
		cli.WriteUsage(stderr)
	}

	return 1
}
