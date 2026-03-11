package cmd

import "fmt"

// Run parses top-level CLI args and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Println("unknown command")
		return 1
	}

	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "sync":
		return runSync(args[1:])
	default:
		fmt.Printf("unknown command: %s\n", args[0])
		return 1
	}
}
