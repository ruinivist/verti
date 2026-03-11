package verti

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadConfig(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		want          Config
		wantErr       bool
		wantArtifacts []string
	}{
		{
			name: "empty artifacts",
			content: `[verti]
repo_id = "repo-1"
artifacts = []
`,
			want: Config{
				RepoID:    "repo-1",
				Artifacts: []string{},
			},
		},
		{
			name: "multiple artifacts",
			content: `[verti]
repo_id = "repo-2"
artifacts = ["test.md", "docs", "build/output.txt"]
`,
			want: Config{
				RepoID:    "repo-2",
				Artifacts: []string{"test.md", "docs", "build/output.txt"},
			},
		},
		{
			name: "missing artifacts defaults empty",
			content: `[verti]
repo_id = "repo-3"
`,
			want: Config{
				RepoID:    "repo-3",
				Artifacts: []string{},
			},
		},
		{
			name: "missing verti table",
			content: `repo_id = "repo-4"
artifacts = []
`,
			wantErr: true,
		},
		{
			name: "empty repo id",
			content: `[verti]
repo_id = ""
artifacts = []
`,
			wantErr: true,
		},
		{
			name: "invalid toml",
			content: `[verti]
repo_id = "repo-5"
artifacts = [
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, tt.content)

			got, err := ReadConfig(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ReadConfig() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadConfig() error = %v", err)
			}
			if got.RepoID != tt.want.RepoID {
				t.Fatalf("ReadConfig() RepoID = %q, want %q", got.RepoID, tt.want.RepoID)
			}
			if !reflect.DeepEqual(got.Artifacts, tt.want.Artifacts) {
				t.Fatalf("ReadConfig() Artifacts = %#v, want %#v", got.Artifacts, tt.want.Artifacts)
			}
		})
	}
}

func TestReadConfigMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	if _, err := ReadConfig(path); err == nil {
		t.Fatal("ReadConfig() error = nil, want error")
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "verti.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
