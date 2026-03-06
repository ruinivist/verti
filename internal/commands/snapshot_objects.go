package commands

import (
	"io"
	"os"
	"path/filepath"

	"verti/internal/artifacts"
	"verti/internal/logging"
	"verti/internal/store"
)

var writeObjectFn = store.WriteObject

func writeManifestObjects(repoRoot, storeRoot, repoID string, maxFileSizeMB int, entries []artifacts.ManifestEntry, stderr io.Writer) {
	maxFileBytes := int64(maxFileSizeMB) * 1024 * 1024
	objectsDir := filepath.Join(storeRoot, "repos", repoID, "objects")

	for i := range entries {
		entry := &entries[i]
		if entry.Kind != artifacts.ArtifactKindFile || entry.Status != artifacts.ArtifactStatusPresent {
			continue
		}

		if entry.Size > maxFileBytes {
			entry.Status = artifacts.ArtifactStatusSkipped
			entry.Hash = ""
			logging.Warnf(stderr, "warning: skipping artifact %q: size %d bytes exceeds max_file_size_mb=%d", entry.Path, entry.Size, maxFileSizeMB)
			continue
		}

		filePath := filepath.Join(repoRoot, filepath.FromSlash(entry.Path))
		data, err := os.ReadFile(filePath)
		if err != nil {
			logging.Warnf(stderr, "warning: unable to read artifact file %q for object store write: %v", entry.Path, err)
			continue
		}

		if _, err := writeObjectFn(objectsDir, data); err != nil {
			logging.Warnf(stderr, "warning: unable to write object for artifact %q: %v", entry.Path, err)
			continue
		}
	}
}
