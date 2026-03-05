package logging

import (
	"fmt"
	"io"
)

// Warnf writes a newline-terminated warning line when a writer is configured.
func Warnf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format+"\n", args...)
}
