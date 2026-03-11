package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const (
	configPath    = ".git/config.toml"
	hookPath      = ".git/hooks/reference-transaction"
	trackedFile   = "test.md"
	storageSubdir = ".verti/repos"
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

	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("failed to resolve executable: %v\n", err)
		os.Exit(1)
	}

	if err := writeReferenceTransactionHook(exePath); err != nil {
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
	if err := ensureGitDir(); err != nil {
		fmt.Println("not a git repository")
		os.Exit(1)
	}

	if _, err := os.Stat(trackedFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("test.md not found")
			os.Exit(1)
		}
		fmt.Printf("failed to check test.md: %v\n", err)
		os.Exit(1)
	}

	repoID, err := readRepoID(configPath)
	if err != nil {
		fmt.Printf("failed to read repo_id: %v\n", err)
		os.Exit(1)
	}

	head, err := gitHead()
	if err != nil {
		fmt.Printf("failed to get head: %v\n", err)
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("failed to resolve home: %v\n", err)
		os.Exit(1)
	}

	repoStoreDir := filepath.Join(home, storageSubdir, repoID)
	if err := os.MkdirAll(repoStoreDir, 0o755); err != nil {
		fmt.Printf("failed to create repo store: %v\n", err)
		os.Exit(1)
	}

	snapshotPath := filepath.Join(repoStoreDir, head)
	_, err = os.Stat(snapshotPath)
	if err == nil {
		if err := copyFile(snapshotPath, trackedFile); err != nil {
			fmt.Printf("failed to restore snapshot: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("restore %s\n", head)
		return
	}
	if !errors.Is(err, os.ErrNotExist) {
		fmt.Printf("failed to check snapshot: %v\n", err)
		os.Exit(1)
	}

	if err := copyFile(trackedFile, snapshotPath); err != nil {
		fmt.Printf("failed to write snapshot: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("snapshot %s\n", head)
}

func ensureGitDir() error {
	gitDirInfo, err := os.Stat(".git")
	if err != nil || !gitDirInfo.IsDir() {
		return errors.New("missing .git")
	}
	return nil
}

func writeReferenceTransactionHook(exePath string) error {
	content := fmt.Sprintf(`#!/bin/sh
state="$1"
if [ "$state" != "committed" ]; then
  exit 0
fi

in_rebase=0
if [ -d .git/rebase-merge ] || [ -d .git/rebase-apply ]; then
  in_rebase=1
fi

zero=0000000000000000000000000000000000000000
trigger=0
while IFS=' ' read -r old new ref; do
  case "$ref" in
    HEAD)
      if [ "$in_rebase" -ne 1 ] && [ "$new" != "$zero" ]; then
        trigger=1
        break
      fi
      ;;
    refs/heads/*)
      if [ "$old" != "$zero" ] && [ "$new" != "$zero" ]; then
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
"%s" sync
`, exePath)
	return os.WriteFile(hookPath, []byte(content), 0o755)
}

func readRepoID(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "repo_id") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return "", errors.New("invalid repo_id line")
		}

		value := strings.TrimSpace(parts[1])
		if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
			return "", errors.New("repo_id must be quoted")
		}
		repoID := value[1 : len(value)-1]
		if repoID == "" {
			return "", errors.New("empty repo_id")
		}
		return repoID, nil
	}

	return "", errors.New("repo_id not found")
}

func gitHead() (string, error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func copyFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, content, 0o644)
}
