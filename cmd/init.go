package cmd

import (
	"fmt"
	"os"
	"strings"

	"verti/internal/verti"
)

func runInit(args []string) int {
	if len(args) != 0 {
		fmt.Printf("unknown init option: %s\n", strings.Join(args, " "))
		return 1
	}

	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("failed to resolve executable: %v\n", err)
		return 1
	}

	if err := verti.Init(exePath); err != nil {
		fmt.Printf("%v\n", err)
		return 1
	}

	fmt.Println("init")
	return 0
}
