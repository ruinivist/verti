package hooks

import (
	"strings"
	"testing"
)

func TestReferenceTransactionDispatcherContainsMarkerSyncAndLegacyPassthrough(t *testing.T) {
	script, err := DispatcherTemplate(ReferenceTransactionHook, "/abs/path/verti", "/abs/path/legacy")
	if err != nil {
		t.Fatalf("DispatcherTemplate() error = %v", err)
	}

	if !strings.Contains(script, "# verti-hooks") {
		t.Fatalf("script missing dispatcher marker:\n%s", script)
	}
	if !strings.Contains(script, "STATE=\"${1:-}\"") {
		t.Fatalf("script missing state parsing:\n%s", script)
	}
	if !strings.Contains(script, "if [ \"${VERTI_BYPASS:-0}\" = \"0\" ] && [ \"$STATE\" = \"committed\" ]; then") {
		t.Fatalf("script missing committed-state guard:\n%s", script)
	}
	if !strings.Contains(script, "\"$VERTI_BIN\" sync --debounced || true") {
		t.Fatalf("script missing sync call:\n%s", script)
	}
	if !strings.Contains(script, "printf '%s' \"$TRANSACTION_INPUT\" | \"$LEGACY_HOOK\" \"$@\" || true") {
		t.Fatalf("script missing best-effort legacy passthrough with stdin forwarding:\n%s", script)
	}
}

func TestDispatcherTemplateRejectsUnsupportedHook(t *testing.T) {
	_, err := DispatcherTemplate("post-commit", "/abs/path/verti", "/abs/path/legacy")
	if err == nil {
		t.Fatalf("expected unsupported hook error")
	}
}
