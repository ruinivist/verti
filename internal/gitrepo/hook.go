package gitrepo

import (
	"bytes"
	_ "embed"
	"os"
	"text/template"
)

//go:embed reference-transaction-hook.sh.tmpl
var referenceTransactionHookTemplate string

//go:embed post-checkout-hook.sh.tmpl
var postCheckoutHookTemplate string

var referenceTransactionTemplate = template.Must(template.New("reference-transaction").Parse(referenceTransactionHookTemplate))
var postCheckoutTemplate = template.Must(template.New("post-checkout").Parse(postCheckoutHookTemplate))

func WriteReferenceTransactionHook(path, executablePath string) error {
	return writeHook(referenceTransactionTemplate, path, executablePath)
}

func WritePostCheckoutHook(path, executablePath string) error {
	return writeHook(postCheckoutTemplate, path, executablePath)
}

func writeHook(tmpl *template.Template, path, executablePath string) error {
	var content bytes.Buffer
	if err := tmpl.Execute(&content, struct {
		ExecutablePath string
	}{ExecutablePath: executablePath}); err != nil {
		return err
	}

	return os.WriteFile(path, content.Bytes(), 0o755)
}
