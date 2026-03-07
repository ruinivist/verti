package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/google/uuid"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("unknown command")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		runInit()
	case "sync":
		fmt.Printf("sync\n")
	default:
		fmt.Printf("unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runInit() {
	gitDirInfo, err := os.Stat(".git")
	if err != nil || !gitDirInfo.IsDir() {
		fmt.Println("not a git repository")
		os.Exit(1)
	}

	configPath := ".git/config.toml"
	_, err = os.Stat(configPath)
	if err == nil {
		fmt.Println("verti already initialized")
		os.Exit(1)
	}
	if !errors.Is(err, os.ErrNotExist) {
		fmt.Printf("failed to check config: %v\n", err)
		os.Exit(1)
	}

	repoID := uuid.NewString()
	content := fmt.Sprintf("[verti]\nrepo_id = %q\n", repoID)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		fmt.Printf("failed to write config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("init\n")
}
