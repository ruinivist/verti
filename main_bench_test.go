package main

import (
	"io"
	"testing"

	"verti/internal/cli"
)

func BenchmarkRunDispatchListNoop(b *testing.B) {
	handlers := cli.Handlers{
		Init:     func(_ []string) error { return nil },
		Snapshot: func(_ []string) error { return nil },
		Restore:  func(_ []string) error { return nil },
		List:     func(_ []string) error { return nil },
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		exitCode := run([]string{"list"}, io.Discard, io.Discard, handlers)
		if exitCode != 0 {
			b.Fatalf("run() exit code = %d, want 0", exitCode)
		}
	}
}
