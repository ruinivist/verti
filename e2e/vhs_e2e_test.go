package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const cliBuildTarget = "./cmd/verti"

type scenario struct {
	name   string
	tape   string
	golden string
	ascii  string
}

func TestVHSE2E(t *testing.T) {
	root := repoRoot(t)
	bin := buildBinary(t, root)
	scenarios := discoverScenarios(t, root)

	for _, tc := range scenarios {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			home := filepath.Join(base, "home")
			repo := filepath.Join(base, "test-repo")

			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatalf("mkdir home: %v", err)
			}

			setupTestRepo(t, root, bin, repo, home)
			runVHS(t, root, bin, tc.tape, repo, home)

			got := readFile(t, tc.ascii)
			want := readFile(t, tc.golden)

			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("ascii output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	return filepath.Dir(wd)
}

func buildBinary(t *testing.T, root string) string {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "verti")

	cmd := exec.Command("go", "build", "-o", bin, cliBuildTarget)
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build cli: %v\n%s", err, out)
	}

	return bin
}

func discoverScenarios(t *testing.T, root string) []scenario {
	t.Helper()

	tapes, err := filepath.Glob(filepath.Join(root, "e2e", "tests", "*.tape"))
	if err != nil {
		t.Fatalf("glob tapes: %v", err)
	}
	if len(tapes) == 0 {
		t.Fatal("no e2e tapes found")
	}

	slices.Sort(tapes)

	scenarios := make([]scenario, 0, len(tapes))
	for _, tape := range tapes {
		name := strings.TrimSuffix(filepath.Base(tape), filepath.Ext(tape))
		scenarios = append(scenarios, scenario{
			name:   name,
			tape:   tape,
			golden: filepath.Join(root, "e2e", "tests", name+".golden.ascii"),
			ascii:  filepath.Join(root, "e2e", "tests", "artifacts", name+".ascii"),
		})
	}

	return scenarios
}

func setupTestRepo(t *testing.T, root, bin, repo, home string) {
	t.Helper()

	cmd := exec.Command(filepath.Join(root, "scripts", "test-repo.sh"))
	cmd.Dir = root
	cmd.Env = withEnv(os.Environ(), map[string]string{
		"GIT_EDITOR":    "true",
		"HOME":          home,
		"TEST_REPO_DIR": repo,
		"VERTI_BIN":     bin,
	})
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("setup test repo: %v\n%s", err, out)
	}
}

func runVHS(t *testing.T, root, bin, tape, repo, home string) {
	t.Helper()

	cmd := exec.Command("vhs", tape)
	cmd.Dir = root
	cmd.Env = withEnv(os.Environ(), map[string]string{
		"E2E_TEST_REPO": repo,
		"GIT_EDITOR":    "true",
		"HOME":          home,
		"PATH":          filepath.Dir(bin) + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run vhs %s: %v\n%s", tape, err, out)
	}
}

func withEnv(base []string, overrides map[string]string) []string {
	env := slices.Clone(base)
	for key, value := range overrides {
		prefix := key + "="
		replaced := false
		for i, entry := range env {
			if strings.HasPrefix(entry, prefix) {
				env[i] = prefix + value
				replaced = true
				break
			}
		}
		if !replaced {
			env = append(env, prefix+value)
		}
	}
	return env
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}
