package cmd

import (
	"fmt"
	"strings"
)

func runInit(args []string) int {
	if len(args) != 0 {
		fmt.Printf("unknown init option: %s\n", strings.Join(args, " "))
		return 1
	}

	fmt.Println("init")
	return 0
}
