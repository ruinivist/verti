package main

import (
	"os"

	"verti/cmd"
)

func main() {
	os.Exit(cmd.Run(os.Args[1:]))
}
