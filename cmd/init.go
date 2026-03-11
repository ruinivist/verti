package cmd

import (
	"os"
	"strings"

	"verti/internal/output"
	"verti/internal/verti"
)

func runInit(args []string) int {
	if len(args) != 0 {
		output.Printf("unknown init option: %s\n", strings.Join(args, " "))
		return 1
	}

	exePath, err := os.Executable()
	if err != nil {
		output.Printf("failed to resolve executable: %v\n", err)
		return 1
	}

	if err := verti.Init(exePath); err != nil {
		output.Printf("%v\n", err)
		return 1
	}

	output.Println("Done initialising...")
	return 0
}
