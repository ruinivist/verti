package snapshots

import "testing"

func TestSnapshotIDBuildAndParseRoundTrip(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	id, err := SnapshotID("feature/branch", sha)
	if err != nil {
		t.Fatalf("SnapshotID() error = %v", err)
	}

	_, gotSHA, ok := ParseSnapshotID(id)
	if !ok {
		t.Fatalf("ParseSnapshotID(%q) ok = false", id)
	}
	if gotSHA != sha {
		t.Fatalf("ParseSnapshotID(%q) sha = %q, want %q", id, gotSHA, sha)
	}
}

func TestSnapshotIDRejectsInvalidInputs(t *testing.T) {
	if _, err := SnapshotID("", "0123456789abcdef0123456789abcdef01234567"); err == nil {
		t.Fatalf("expected empty branch identity to fail")
	}
	if _, err := SnapshotID("main", "abc123"); err == nil {
		t.Fatalf("expected short sha to fail")
	}
}

func TestParseSnapshotIDRejectsOldLayoutAndInvalidValues(t *testing.T) {
	tests := []string{
		"abc123",
		"main--abc123",
		"--0123456789abcdef0123456789abcdef01234567",
		"branch--zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
	}

	for _, tc := range tests {
		if _, _, ok := ParseSnapshotID(tc); ok {
			t.Fatalf("ParseSnapshotID(%q) ok = true, want false", tc)
		}
	}
}
