package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"verti/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, cli.NotImplementedHandlers()))
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
