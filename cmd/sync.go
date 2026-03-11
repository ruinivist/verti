package cmd

import (
	"fmt"
	"strings"
)

func runSync(args []string) int {
	if len(args) != 0 {
		fmt.Printf("unknown sync option: %s\n", strings.Join(args, " "))
		return 1
	}

	fmt.Println("sync")
	return 0
}
