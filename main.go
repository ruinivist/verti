package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("unknown command")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		fmt.Printf("init\n")
	case "sync":
		fmt.Printf("sync\n")
	default:
		fmt.Printf("unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
