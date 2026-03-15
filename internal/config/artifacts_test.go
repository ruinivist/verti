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
			name:  "keeps relative file",
			input: "docs/guide.md",
			want:  "docs/guide.md",
		},
		{
			name:  "strips leading slash",
			input: "/docs/guide.md",
			want:  "docs/guide.md",
		},
		{
			name:  "preserves trailing slash",
			input: "docs/",
			want:  "docs/",
		},
		{
			name:  "strips leading slash and preserves trailing slash",
			input: "/docs/",
			want:  "docs/",
		},
		{
			name:    "empty",
			input:   "",
			wantErr: "empty path",
		},
		{
			name:    "root only",
			input:   "/",
			wantErr: "must point to a file or directory",
		},
		{
			name:    "multiple leading slashes",
			input:   "//docs",
			wantErr: "must not start with multiple slashes",
		},
		{
			name:    "dot",
			input:   ".",
			wantErr: "must be rooted at repository top",
		},
		{
			name:    "leading dot",
			input:   "./docs",
			wantErr: "must be rooted at repository top",
		},
		{
			name:    "embedded dot",
			input:   "docs/./guide.md",
			wantErr: "must be rooted at repository top",
		},
		{
			name:    "escape",
			input:   "../outside.txt",
			wantErr: "must not escape repository",
		},
		{
			name:    "rooted escape",
			input:   "/../outside.txt",
			wantErr: "must not escape repository",
		},
		{
			name:    "embedded escape",
			input:   "docs/../guide.md",
			wantErr: "must not escape repository",
		},
		{
			name:    "empty segment",
			input:   "docs//guide.md",
			wantErr: "must not contain empty path segments",
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
