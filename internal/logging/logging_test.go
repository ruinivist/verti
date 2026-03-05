package logging

import (
	"bytes"
	"testing"
)

func TestWarnfWritesNewlineTerminatedMessage(t *testing.T) {
	var out bytes.Buffer

	Warnf(&out, "warning: %s", "issue")

	if got, want := out.String(), "warning: issue\n"; got != want {
		t.Fatalf("Warnf() output = %q, want %q", got, want)
	}
}

func TestWarnfNilWriterIsNoOp(t *testing.T) {
	Warnf(nil, "warning: %s", "issue")
}
