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
	configPath  = ".git/verti.toml"
	hookPath    = ".git/hooks/reference-transaction"
	excludePath = ".git/info/exclude"
)

//go:embed reference-transaction-hook.sh.tmpl
var referenceTransactionHookTemplate string

var hookTemplate = template.Must(template.New("reference-transaction").Parse(referenceTransactionHookTemplate))

func Init(exePath string) error {
	if err := ensureGitDir(); err != nil {
		return errors.New("not a git repository")
	}

	if _, err := os.Stat(configPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to check config: %v", err)
		}
		if err := WriteConfig(configPath, Config{
			RepoID:    uuid.NewString(),
			Artifacts: []string{},
		}); err != nil {
			return fmt.Errorf("failed to write config: %v", err)
		}
	}

	if err := writeReferenceTransactionHook(exePath); err != nil {
		return fmt.Errorf("failed to write hook: %v", err)
	}

	if err := openEditor(configPath); err != nil {
		return fmt.Errorf("failed to open editor: %v", err)
	}

	cfg, err := ReadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	if err := gitExcludeArtifacts(excludePath, cfg.Artifacts); err != nil {
		return fmt.Errorf("failed to update exclude: %v", err)
	}

	return nil
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
