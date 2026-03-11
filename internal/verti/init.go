package verti

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"text/template"

	"github.com/google/uuid"
)

const (
	configPath = ".git/verti.toml"
	hookPath   = ".git/hooks/reference-transaction"
)

//go:embed reference-transaction-hook.sh.tmpl
var referenceTransactionHookTemplate string

var hookTemplate = template.Must(template.New("reference-transaction").Parse(referenceTransactionHookTemplate))

func Init(exePath string) error {
	if err := ensureGitDir(); err != nil {
		return errors.New("not a git repository")
	}

	_, err := os.Stat(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check config: %v", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		if err := writeConfig(configPath); err != nil {
			return fmt.Errorf("failed to write config: %v", err)
		}
	}

	if err := writeReferenceTransactionHook(exePath); err != nil {
		return fmt.Errorf("failed to write hook: %v", err)
	}

	return nil
}

func writeConfig(path string) error {
	content := fmt.Sprintf("[verti]\nrepo_id = %q\nartifacts = []\n", uuid.NewString())
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeReferenceTransactionHook(exePath string) error {
	var content bytes.Buffer
	if err := hookTemplate.Execute(&content, struct {
		ExecutablePath string
	}{ExecutablePath: exePath}); err != nil {
		return err
	}

	return os.WriteFile(hookPath, content.Bytes(), 0o755)
}
