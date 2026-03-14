package config

import "testing"

func TestNormalizeArtifactPath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{
			name:  "clean relative file",
			input: "docs/guide.md",
			want:  "docs/guide.md",
		},
		{
			name:  "cleans nested path",
			input: "docs/../notes.txt",
			want:  "notes.txt",
		},
		{
			name:    "empty",
			input:   "",
			wantErr: "empty path",
		},
		{
			name:    "absolute",
			input:   "/tmp/file.txt",
			wantErr: "must be relative",
		},
		{
			name:    "dot",
			input:   ".",
			wantErr: "must point to a file",
		},
		{
			name:    "escape",
			input:   "../outside.txt",
			wantErr: "must not escape repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeArtifactPath(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("NormalizeArtifactPath() error = nil, want error")
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("NormalizeArtifactPath() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeArtifactPath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeArtifactPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
