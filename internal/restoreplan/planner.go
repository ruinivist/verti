package restoreplan

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"verti/internal/artifacts"
)

type OperationType string

const (
	OpMkdir       OperationType = "mkdir"
	OpWriteFile   OperationType = "write_file"
	OpWriteSymlink OperationType = "write_symlink"
	OpRemove      OperationType = "remove"
)

type Operation struct {
	Type       OperationType
	Path       string
	AbsPath    string
	LinkTarget string
}

// BuildPlan builds a deterministic restore plan and validates path safety/collisions.
// targetEntries come from the target snapshot manifest; currentPaths are repo-relative
// currently-present paths considered for stale removal.
func BuildPlan(repoRoot string, targetEntries []artifacts.ManifestEntry, currentPaths []string) ([]Operation, error) {
	repoRootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root %q: %w", repoRoot, err)
	}

	normalizedTarget := make([]artifacts.ManifestEntry, 0, len(targetEntries))
	presentKinds := map[string]string{} // path -> "dir"|"nondir"

	for _, e := range targetEntries {
		p, abs, err := normalizeAndValidate(repoRootAbs, e.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid target entry path %q: %w", e.Path, err)
		}
		e.Path = p

		if e.Status == artifacts.ArtifactStatusPresent {
			switch e.Kind {
			case artifacts.ArtifactKindDir:
				if err := registerPresentKind(presentKinds, p, "dir"); err != nil {
					return nil, err
				}
			case artifacts.ArtifactKindFile, artifacts.ArtifactKindSymlink:
				if err := registerPresentKind(presentKinds, p, "nondir"); err != nil {
					return nil, err
				}
			case artifacts.ArtifactKindMissing:
				// Ignore inconsistent combination and handle as non-present path.
			default:
				return nil, fmt.Errorf("unsupported present manifest kind %q for path %q", e.Kind, p)
			}
		}

		_ = abs
		normalizedTarget = append(normalizedTarget, e)
	}

	if err := validateCollisions(presentKinds); err != nil {
		return nil, err
	}

	keep := make(map[string]struct{})
	var mkdirOps, fileOps, symlinkOps []Operation

	for _, e := range normalizedTarget {
		p := e.Path
		abs := filepath.Join(repoRootAbs, filepath.FromSlash(p))

		if e.Status != artifacts.ArtifactStatusPresent {
			continue
		}

		keep[p] = struct{}{}
		addAncestorDirs(keep, p)

		switch e.Kind {
		case artifacts.ArtifactKindDir:
			mkdirOps = append(mkdirOps, Operation{Type: OpMkdir, Path: p, AbsPath: abs})
		case artifacts.ArtifactKindFile:
			fileOps = append(fileOps, Operation{Type: OpWriteFile, Path: p, AbsPath: abs})
		case artifacts.ArtifactKindSymlink:
			symlinkOps = append(symlinkOps, Operation{Type: OpWriteSymlink, Path: p, AbsPath: abs, LinkTarget: e.LinkTarget})
		}
	}

	sortMkdirOps(mkdirOps)
	sortByPath(fileOps)
	sortByPath(symlinkOps)

	removeSet := make(map[string]struct{})
	for _, raw := range currentPaths {
		p, _, err := normalizeAndValidate(repoRootAbs, raw)
		if err != nil {
			return nil, fmt.Errorf("invalid current path %q: %w", raw, err)
		}
		if _, shouldKeep := keep[p]; shouldKeep {
			continue
		}
		removeSet[p] = struct{}{}
	}

	removeOps := make([]Operation, 0, len(removeSet))
	for p := range removeSet {
		removeOps = append(removeOps, Operation{
			Type:    OpRemove,
			Path:    p,
			AbsPath: filepath.Join(repoRootAbs, filepath.FromSlash(p)),
		})
	}
	sortRemoveOps(removeOps)

	out := make([]Operation, 0, len(mkdirOps)+len(fileOps)+len(symlinkOps)+len(removeOps))
	out = append(out, mkdirOps...)
	out = append(out, fileOps...)
	out = append(out, symlinkOps...)
	out = append(out, removeOps...)
	return out, nil
}

func registerPresentKind(kinds map[string]string, p, kind string) error {
	if existing, ok := kinds[p]; ok && existing != kind {
		return fmt.Errorf("restore plan collision at %q: both file/symlink and directory targets exist", p)
	}
	kinds[p] = kind
	return nil
}

func validateCollisions(kinds map[string]string) error {
	paths := make([]string, 0, len(kinds))
	for p := range kinds {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		parent := path.Dir(p)
		for parent != "." && parent != "/" {
			if kind, ok := kinds[parent]; ok && kind != "dir" {
				return fmt.Errorf("restore plan collision: %q is a file/symlink but also parent of %q", parent, p)
			}
			parent = path.Dir(parent)
		}
	}

	for _, p := range paths {
		if kinds[p] == "dir" {
			continue
		}
		prefix := p + "/"
		for _, q := range paths {
			if strings.HasPrefix(q, prefix) {
				return fmt.Errorf("restore plan collision: %q is a file/symlink but %q also targets beneath it", p, q)
			}
		}
	}

	return nil
}

func normalizeAndValidate(repoRootAbs, rawPath string) (string, string, error) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(p) {
		return "", "", fmt.Errorf("path %q is absolute", rawPath)
	}

	cleaned := filepath.Clean(filepath.FromSlash(p))
	if cleaned == "." || cleaned == "" {
		return "", "", fmt.Errorf("path %q is not a concrete repo-relative target", rawPath)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path %q escapes repo root via '..'", rawPath)
	}

	abs := filepath.Join(repoRootAbs, cleaned)
	rel, err := filepath.Rel(repoRootAbs, abs)
	if err != nil {
		return "", "", fmt.Errorf("resolve relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path %q escapes repo root", rawPath)
	}

	return filepath.ToSlash(cleaned), abs, nil
}

func addAncestorDirs(keep map[string]struct{}, p string) {
	parent := path.Dir(p)
	for parent != "." && parent != "/" {
		keep[parent] = struct{}{}
		parent = path.Dir(parent)
	}
}

func sortByPath(ops []Operation) {
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].Path < ops[j].Path
	})
}

func sortMkdirOps(ops []Operation) {
	sort.Slice(ops, func(i, j int) bool {
		di := pathDepth(ops[i].Path)
		dj := pathDepth(ops[j].Path)
		if di != dj {
			return di < dj
		}
		return ops[i].Path < ops[j].Path
	})
}

func sortRemoveOps(ops []Operation) {
	sort.Slice(ops, func(i, j int) bool {
		di := pathDepth(ops[i].Path)
		dj := pathDepth(ops[j].Path)
		if di != dj {
			return di > dj // deeper paths first for safe delete ordering
		}
		return ops[i].Path > ops[j].Path
	})
}

func pathDepth(p string) int {
	if p == "" {
		return 0
	}
	return strings.Count(p, "/") + 1
}
