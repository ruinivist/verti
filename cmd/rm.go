package cmd

import (
	"os"
	"strings"

	"verti/internal/output"
	"verti/internal/verti"
)

func runRemove(args []string) int {
	if len(args) == 0 {
		output.Println("usage: verti rm <path>")
		return 1
	}
	if len(args) > 1 {
		output.Printf("unknown rm option: %s\n", strings.Join(args[1:], " "))
		return 1
	}

	exePath, err := os.Executable()
	if err != nil {
		output.Printf("failed to resolve executable: %v\n", err)
		return 1
	}

	if err := verti.Remove(exePath, args[0]); err != nil {
		output.Printf("%v\n", err)
		return 1
	}

	return 0
}
