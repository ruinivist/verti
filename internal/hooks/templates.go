package hooks

import (
	"embed"
	"fmt"
	"path"
	"strconv"
	"strings"
	"text/template"
)

//go:embed templates/*.sh.tmpl
var dispatcherTemplates embed.FS

const (
	ReferenceTransactionHook = "reference-transaction"
)

var hookTemplateFile = map[string]string{
	ReferenceTransactionHook: "reference_transaction.sh.tmpl",
}

// DispatcherTemplate returns the bash dispatcher script for the given hook.
func DispatcherTemplate(hookName, vertiBinPath, legacyHookPath string) (string, error) {
	templateName, ok := hookTemplateFile[hookName]
	if !ok {
		return "", fmt.Errorf("unsupported hook %q", hookName)
	}

	raw, err := dispatcherTemplates.ReadFile(path.Join("templates", templateName))
	if err != nil {
		return "", fmt.Errorf("read hook template %q: %w", templateName, err)
	}

	tmpl, err := template.New(templateName).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse hook template %q: %w", templateName, err)
	}

	var b strings.Builder
	data := struct {
		VertiBinPath   string
		LegacyHookPath string
	}{
		VertiBinPath:   strconv.Quote(vertiBinPath),
		LegacyHookPath: strconv.Quote(legacyHookPath),
	}

	if err := tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render hook template %q: %w", templateName, err)
	}

	return b.String(), nil
}
