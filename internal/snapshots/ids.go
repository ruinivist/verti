package snapshots

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const DetachedBranchIdentity = "__detached__"

// SnapshotID returns the branch-scoped snapshot id "<branchKey>--<sha>".
func SnapshotID(branchIdentity, commitSHA string) (string, error) {
	branchIdentity = strings.TrimSpace(branchIdentity)
	if branchIdentity == "" {
		return "", fmt.Errorf("branch identity cannot be empty")
	}

	commitSHA = strings.TrimSpace(strings.ToLower(commitSHA))
	if !isFullSHA(commitSHA) {
		return "", fmt.Errorf("invalid commit sha %q", commitSHA)
	}

	branchKey := base64.RawURLEncoding.EncodeToString([]byte(branchIdentity))
	return branchKey + "--" + commitSHA, nil
}

// ParseSnapshotID parses a branch-scoped snapshot id and validates its shape.
func ParseSnapshotID(id string) (branchKey string, commitSHA string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(id), "--", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	branchKey = strings.TrimSpace(parts[0])
	commitSHA = strings.TrimSpace(strings.ToLower(parts[1]))
	if branchKey == "" || !isFullSHA(commitSHA) {
		return "", "", false
	}
	return branchKey, commitSHA, true
}

func isFullSHA(sha string) bool {
	if len(sha) != 40 {
		return false
	}
	for i := 0; i < len(sha); i++ {
		c := sha[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}
