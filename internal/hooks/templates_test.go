package hooks

import (
	"strings"
	"testing"
)

func TestSnapshotDispatchersContainMarkerSnapshotAndLegacyPassthrough(t *testing.T) {
	tests := []struct {
		name string
		hook string
	}{
		{name: "post_commit", hook: "post-commit"},
		{name: "post_merge", hook: "post-merge"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			script, err := DispatcherTemplate(tc.hook, "/abs/path/verti", "/abs/path/legacy")
			if err != nil {
				t.Fatalf("DispatcherTemplate() error = %v", err)
			}

				if !strings.Contains(script, "# verti-hooks") {
					t.Fatalf("script missing dispatcher marker:\n%s", script)
				}
			if !strings.Contains(script, "\"$VERTI_BIN\" snapshot || true") {
				t.Fatalf("script missing snapshot call:\n%s", script)
			}
			if !strings.Contains(script, "[ -x \"$LEGACY_HOOK\" ] && \"$LEGACY_HOOK\" \"$@\"") {
				t.Fatalf("script missing legacy passthrough:\n%s", script)
			}
		})
	}
}

func TestCheckoutDispatcherRestoresOnlyForCommitChangingBranchCheckout(t *testing.T) {
	script, err := DispatcherTemplate("post-checkout", "/abs/path/verti", "/abs/path/legacy")
	if err != nil {
		t.Fatalf("DispatcherTemplate() error = %v", err)
	}

	if !strings.Contains(script, "if [ \"${3:-0}\" = \"1\" ] && [ \"${1:-}\" != \"${2:-}\" ]; then") {
		t.Fatalf("checkout script missing commit-changing guard:\n%s", script)
	}
	if !strings.Contains(script, "\"$VERTI_BIN\" restore \"${2}\" || true") {
		t.Fatalf("checkout script missing restore call for new sha:\n%s", script)
	}
}

func TestRewriteDispatcherCapturesAndForwardsStdinToLegacyHook(t *testing.T) {
	script, err := DispatcherTemplate("post-rewrite", "/abs/path/verti", "/abs/path/legacy")
	if err != nil {
		t.Fatalf("DispatcherTemplate() error = %v", err)
	}

	if !strings.Contains(script, "REWRITE_INPUT=\"$(cat)\"") {
		t.Fatalf("rewrite script missing stdin capture:\n%s", script)
	}
	if !strings.Contains(script, "\"$VERTI_BIN\" snapshot || true") {
		t.Fatalf("rewrite script missing snapshot call:\n%s", script)
	}
	if !strings.Contains(script, "printf '%s' \"$REWRITE_INPUT\" | \"$LEGACY_HOOK\" \"$@\"") {
		t.Fatalf("rewrite script missing stdin forwarding to legacy hook:\n%s", script)
	}
}
