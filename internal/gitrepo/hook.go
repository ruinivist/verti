package gitrepo

import (
	"bytes"
	_ "embed"
	"os"
	"text/template"
)

//go:embed reference-transaction-hook.sh.tmpl
var referenceTransactionHookTemplate string

var hookTemplate = template.Must(template.New("reference-transaction").Parse(referenceTransactionHookTemplate))

func WriteReferenceTransactionHook(path, executablePath string) error {
	var content bytes.Buffer
	if err := hookTemplate.Execute(&content, struct {
		ExecutablePath string
	}{ExecutablePath: executablePath}); err != nil {
		return err
	}

	return os.WriteFile(path, content.Bytes(), 0o755)
}
