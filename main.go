package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"verti/internal/cli"
	"verti/internal/commands"
	"verti/internal/reporting"
)

func main() {
	handlers := cli.Handlers{
		Init: newHandler(func(wd string, _ []string) error {
			return commands.RunInit(wd)
		}),
		Snapshot: newHandler(func(wd string, _ []string) error {
			return commands.RunSnapshot(wd)
		}),
		Restore: newHandler(commands.RunRestore),
		List:    newHandler(commands.RunList),
	}

	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, handlers))
}

func newHandler(command func(workingDir string, args []string) error) cli.Handler {
	return func(args []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
		return command(wd, args)
	}
}

func run(args []string, stdout, stderr io.Writer, handlers cli.Handlers) int {
	_ = stdout

	err := cli.Dispatch(args, handlers)
	if err == nil {
		return 0
	}

	fmt.Fprintf(stderr, "verti: %s\n", reporting.Format(err, reporting.DebugEnabled()))

	var usageErr *cli.UsageError
	if errors.As(err, &usageErr) {
		cli.WriteUsage(stderr)
	}

	return 1
}
