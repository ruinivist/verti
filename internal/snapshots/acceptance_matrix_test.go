package snapshots

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMVPAcceptanceCriterion10NoPartialSnapshotVisibleOnPublishFailure(t *testing.T) {
	scopeDir := t.TempDir()
	sha := "ac10-sha"

	origHook := beforePublishRenameHook
	beforePublishRenameHook = func(_, _ string) error {
		return errors.New("ac10 injected publish interruption")
	}
	t.Cleanup(func() { beforePublishRenameHook = origHook })

	_, err := PublishSnapshot(scopeDir, sha, nil, Meta{CommitSHA: sha})
	if err == nil {
		t.Fatalf("PublishSnapshot() expected injected pre-rename failure")
	}

	publishedPath := filepath.Join(scopeDir, "snapshots", sha)
	if _, statErr := os.Stat(publishedPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no visible partial snapshot at %q, stat err=%v", publishedPath, statErr)
	}
}
