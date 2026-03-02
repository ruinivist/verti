package reporting

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestFormatClassifiedErrorsByClass(t *testing.T) {
	cases := []struct {
		name  string
		class Class
		want  string
	}{
		{name: "config", class: ClassConfig, want: "config: load config"},
		{name: "store", class: ClassStore, want: "store: write object"},
		{name: "restore", class: ClassRestore, want: "restore: apply restore plan"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Wrap(tc.class, strings.SplitN(tc.want, ": ", 2)[1], errors.New("cause"))
			got := Format(err, false)
			if got != tc.want {
				t.Fatalf("Format(debug=false) = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatDebugIncludesCauseNormalModeConcise(t *testing.T) {
	base := errors.New("permission denied")
	err := Wrap(ClassStore, "publish snapshot", base)

	concise := Format(err, false)
	if concise != "store: publish snapshot" {
		t.Fatalf("concise format = %q, want %q", concise, "store: publish snapshot")
	}

	debug := Format(err, true)
	if !strings.Contains(debug, "store: publish snapshot") {
		t.Fatalf("debug format missing primary message: %q", debug)
	}
	if !strings.Contains(debug, "permission denied") {
		t.Fatalf("debug format missing underlying cause: %q", debug)
	}
}

func TestDebugEnabledFromEnv(t *testing.T) {
	t.Setenv("VERTI_DEBUG", "1")
	if !DebugEnabled() {
		t.Fatalf("DebugEnabled() with VERTI_DEBUG=1 should be true")
	}

	t.Setenv("VERTI_DEBUG", "true")
	if !DebugEnabled() {
		t.Fatalf("DebugEnabled() with VERTI_DEBUG=true should be true")
	}

	t.Setenv("VERTI_DEBUG", "")
	if DebugEnabled() {
		t.Fatalf("DebugEnabled() with empty env should be false")
	}

	if err := os.Unsetenv("VERTI_DEBUG"); err != nil {
		t.Fatalf("unset env: %v", err)
	}
	if DebugEnabled() {
		t.Fatalf("DebugEnabled() with unset env should be false")
	}
}
