package cmd

import (
	"strconv"
	"strings"

	"verti/internal/output"
	"verti/internal/verti"
)

func runOrphans(args []string) int {
	if len(args) == 0 {
		if err := verti.Orphans(); err != nil {
			output.Printf("%v\n", err)
			return 1
		}
		return 0
	}
	if len(args) > 1 {
		output.Printf("unknown orphans option: %s\n", strings.Join(args[1:], " "))
		return 1
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		output.Printf("invalid orphan number: %s\n", args[0])
		return 1
	}

	if err := verti.RestoreOrphan(index); err != nil {
		output.Printf("%v\n", err)
		return 1
	}

	return 0
}
