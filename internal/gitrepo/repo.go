package gitrepo

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func EnsureGitDir() error {
	out, err := gitOutput("rev-parse", "--is-inside-work-tree")
	if err != nil {
		return err
	}
	if out != "true" {
		return fmt.Errorf("missing .git")
	}
	return nil
}

type Paths struct {
	CommonDir                string
	Config                   string
	ReferenceTransactionHook string
	PostCheckoutHook         string
	Exclude                  string
}

func ResolvePaths() (Paths, error) {
	commonDir, err := gitAbsolutePath("rev-parse", "--git-common-dir")
	if err != nil {
		return Paths{}, err
	}

	return Paths{
		CommonDir:                commonDir,
		Config:                   filepath.Join(commonDir, "verti.toml"),
		ReferenceTransactionHook: filepath.Join(commonDir, "hooks", "reference-transaction"),
		PostCheckoutHook:         filepath.Join(commonDir, "hooks", "post-checkout"),
		Exclude:                  filepath.Join(commonDir, "info", "exclude"),
	}, nil
}

func Head() (string, error) {
	out, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}

func HeadDisplay() (string, error) {
	out, err := gitOutput("show", "-s", "--format=%s [%h]", "HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}

func gitAbsolutePath(args ...string) (string, error) {
	path, err := gitOutput(args...)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absPath), nil
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
