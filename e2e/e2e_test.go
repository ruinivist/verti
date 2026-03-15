package e2e_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const cliBuildTarget = "./cmd/verti"

const (
	startInRepo   = "repo"
	startInNoRepo = "no-repo"
)

type scenario struct {
	name   string
	mode   string
	keys   string
	golden string
}

func TestScriptE2E(t *testing.T) {
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

			setupScenario(t, root, bin, tc.mode, repo, home)
			outPath := runScriptReplay(t, root, bin, tc, repo, home, base)

			got := readFile(t, outPath)
			want := readFile(t, tc.golden)

			if got != want {
				t.Fatalf("output mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
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

	for _, pattern := range []string{
		filepath.Join(root, "e2e", "tests", "*.keys"),
		filepath.Join(root, "e2e", "tests", "*.golden.out"),
	} {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		if len(paths) != 0 {
			t.Fatalf("root-level e2e fixtures are not supported; move %s under e2e/tests/repo or e2e/tests/no-repo", filepath.Base(paths[0]))
		}
	}

	scenarios := make([]scenario, 0)
	for _, mode := range []string{startInNoRepo, startInRepo} {
		keysFiles, err := filepath.Glob(filepath.Join(root, "e2e", "tests", mode, "*.keys"))
		if err != nil {
			t.Fatalf("glob %s fixtures: %v", mode, err)
		}
		slices.Sort(keysFiles)

		for _, keys := range keysFiles {
			base := strings.TrimSuffix(filepath.Base(keys), filepath.Ext(keys))
			scenarios = append(scenarios, scenario{
				name:   filepath.Join(mode, base),
				mode:   mode,
				keys:   keys,
				golden: filepath.Join(root, "e2e", "tests", mode, base+".golden.out"),
			})
		}
	}
	if len(scenarios) == 0 {
		t.Fatal("no e2e scenarios found")
	}

	return scenarios
}

func setupScenario(t *testing.T, root, bin, mode, repo, home string) {
	t.Helper()

	if mode == startInNoRepo {
		return
	}
	if mode != startInRepo {
		t.Fatalf("unsupported scenario mode %q", mode)
	}

	cmd := exec.Command(filepath.Join(root, "scripts", "test-repo.sh"))
	cmd.Dir = root
	cmd.Env = withEnv(os.Environ(), map[string]string{
		"HOME":          home,
		"TEST_REPO_DIR": repo,
		"VERTI_BIN":     bin,
	})
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("setup test repo: %v\n%s", err, out)
	}
}

func runScriptReplay(t *testing.T, root, bin string, tc scenario, repo, home, base string) string {
	t.Helper()

	keys := readRawFile(t, tc.keys)
	if bytes.HasSuffix(keys, []byte{'\n'}) {
		keys = keys[:len(keys)-1]
	}
	outBase := strings.ReplaceAll(tc.name, string(os.PathSeparator), "_")
	rawOutPath := filepath.Join(base, outBase+".out.raw")
	outPath := filepath.Join(base, outBase+".out")
	shellPath := filepath.Join(root, "scripts", "e2e-shell.sh")

	cmd := exec.Command("script",
		"-q",
		"-e",
		"-E", "never",
		"-O", rawOutPath,
		"-c", shellPath,
	)
	cmd.Dir = root
	env := map[string]string{
		"E2E_START_IN": tc.mode,
		"HOME":         home,
		"HISTFILE":     "/dev/null",
		"PATH":         filepath.Dir(bin) + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TERM":         "xterm-256color",
	}
	if tc.mode == startInRepo {
		env["E2E_TEST_REPO"] = repo
	}
	cmd.Env = withEnv(os.Environ(), env)
	cmd.Stdin = bytes.NewReader(keys)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run script replay %s: %v\n%s", tc.keys, err, out)
	}

	cleaned := stripScriptWrapper(t, readRawFile(t, rawOutPath))
	if err := os.WriteFile(outPath, cleaned, 0o644); err != nil {
		t.Fatalf("write %s: %v", outPath, err)
	}

	return outPath
}

func stripScriptWrapper(t *testing.T, data []byte) []byte {
	t.Helper()

	start := bytes.IndexByte(data, '\n')
	if start == -1 {
		t.Fatalf("script log missing header newline")
	}
	data = data[start+1:]

	end := bytes.LastIndex(data, []byte("\nScript done on "))
	if end == -1 {
		t.Fatalf("script log missing footer")
	}

	return data[:end+1]
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

	return string(readRawFile(t, path))
}

func readRawFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return data
}
