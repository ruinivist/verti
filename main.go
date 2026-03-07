package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
)

const (
	configPath = ".git/config.toml"
	hookPath   = ".git/hooks/reference-transaction"
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
		runSync(os.Args[2:])
	default:
		fmt.Printf("unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runInit() {
	if err := ensureGitDir(); err != nil {
		fmt.Println("not a git repository")
		os.Exit(1)
	}

	_, err := os.Stat(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Printf("failed to check config: %v\n", err)
		os.Exit(1)
	}
	if errors.Is(err, os.ErrNotExist) {
		repoID := uuid.NewString()
		content := fmt.Sprintf("[verti]\nrepo_id = %q\n", repoID)
		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
			fmt.Printf("failed to write config: %v\n", err)
			os.Exit(1)
		}
	}

	if err := writeReferenceTransactionHook(); err != nil {
		fmt.Printf("failed to write hook: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("init\n")
}

func runSync(args []string) {
	if len(args) == 0 {
		runSyncAction()
		return
	}

	fmt.Printf("unknown sync option: %s\n", strings.Join(args, " "))
	os.Exit(1)
}

func runSyncAction() {
	fmt.Printf("sync\n")
}

func ensureGitDir() error {
	gitDirInfo, err := os.Stat(".git")
	if err != nil || !gitDirInfo.IsDir() {
		return errors.New("missing .git")
	}
	return nil
}

func writeReferenceTransactionHook() error {
	content := `#!/bin/sh
state="$1"
if [ "$state" != "committed" ]; then
  exit 0
fi

trigger=0
while IFS=' ' read -r old new ref; do
  case "$ref" in
    HEAD|refs/heads/*)
      if [ "$new" != "0000000000000000000000000000000000000000" ]; then
        trigger=1
        break
      fi
      ;;
  esac
done

if [ "$trigger" -ne 1 ]; then
  exit 0
fi

printf "reference-transaction committed ref update\n"
verti sync
`
	return os.WriteFile(hookPath, []byte(content), 0o755)
}
