package cmd

import (
	"fmt"
	"strings"

	"verti/internal/verti"
)

func runSync(args []string) int {
	if len(args) != 0 {
		fmt.Printf("unknown sync option: %s\n", strings.Join(args, " "))
		return 1
	}

	if err := verti.Sync(); err != nil {
		fmt.Printf("%v\n", err)
		return 1
	}

	return 0
}
