package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	configPath       = ".git/config.toml"
	hookPath         = ".git/hooks/reference-transaction"
	debouncePath     = ".git/verti.debounce"
	debounceWindowMs = int64(500)
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

	if len(args) == 1 && args[0] == "--debounced" {
		runSyncDebounced()
		return
	}

	fmt.Printf("unknown sync option: %s\n", strings.Join(args, " "))
	os.Exit(1)
}

func runSyncDebounced() {
	if err := ensureGitDir(); err != nil {
		fmt.Println("not a git repository")
		os.Exit(1)
	}

	head, err := gitHead()
	if err != nil {
		fmt.Printf("failed to get head: %v\n", err)
		os.Exit(1)
	}

	lastHead, lastTs, hasState, err := readDebounceState()
	if err != nil {
		fmt.Printf("failed to read debounce state: %v\n", err)
		os.Exit(1)
	}

	now := time.Now().UnixMilli()
	if hasState && lastHead == head && (now-lastTs) < debounceWindowMs {
		fmt.Printf("debounced noop\n")
		return
	}

	fmt.Printf("debounce expired, action done\n")
	runSyncAction()
	if err := writeDebounceState(head, now); err != nil {
		fmt.Printf("failed to write debounce state: %v\n", err)
		os.Exit(1)
	}
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

printf "reference-transaction committed event\n"
verti sync --debounced
`
	return os.WriteFile(hookPath, []byte(content), 0o755)
}

func gitHead() (string, error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func readDebounceState() (string, int64, bool, error) {
	content, err := os.ReadFile(debouncePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", 0, false, nil
		}
		return "", 0, false, err
	}

	parts := strings.Fields(string(content))
	if len(parts) != 2 {
		return "", 0, false, errors.New("invalid debounce state format")
	}

	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, false, err
	}

	return parts[0], ts, true, nil
}

func writeDebounceState(head string, ts int64) error {
	content := fmt.Sprintf("%s %d\n", head, ts)
	return os.WriteFile(debouncePath, []byte(content), 0o644)
}
