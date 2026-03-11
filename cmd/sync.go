package cmd

import (
	"strings"

	"verti/internal/output"
	"verti/internal/verti"
)

func runSync(args []string) int {
	if len(args) != 0 {
		output.Printf("unknown sync option: %s\n", strings.Join(args, " "))
		return 1
	}

	if err := verti.Sync(); err != nil {
		output.Printf("%v\n", err)
		return 1
	}

	return 0
}
