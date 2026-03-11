package gitrepo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func EnsureGitDir() error {
	info, err := os.Stat(".git")
	if err != nil || !info.IsDir() {
		return errors.New("missing .git")
	}
	return nil
}

func Head() (string, error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func HeadDisplay() (string, error) {
	out, err := exec.Command("git", "show", "-s", "--format=%s [%h]", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
