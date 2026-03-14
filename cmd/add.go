package cmd

import (
	"os"
	"strings"

	"verti/internal/output"
	"verti/internal/verti"
)

func runAdd(args []string) int {
	if len(args) == 0 {
		output.Println("usage: verti add <path>")
		return 1
	}
	if len(args) > 1 {
		output.Printf("unknown add option: %s\n", strings.Join(args[1:], " "))
		return 1
	}

	exePath, err := os.Executable()
	if err != nil {
		output.Printf("failed to resolve executable: %v\n", err)
		return 1
	}

	if err := verti.Add(exePath, args[0]); err != nil {
		output.Printf("%v\n", err)
		return 1
	}

	return 0
}
