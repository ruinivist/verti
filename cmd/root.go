package cmd

import "verti/internal/output"

// Run parses top-level CLI args and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		output.Println("unknown command")
		return 1
	}

	switch args[0] {
	case "add":
		return runAdd(args[1:])
	case "init":
		return runInit(args[1:])
	case "rm":
		return runRemove(args[1:])
	case "orphans":
		return runOrphans(args[1:])
	case "sync":
		return runSync(args[1:])
	default:
		output.Printf("unknown command: %s\n", args[0])
		return 1
	}
}
