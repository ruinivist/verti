package store

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteObjectCreatesHashedObjectFile(t *testing.T) {
	objectsDir := t.TempDir()
	data := []byte("hello object store\n")

	hash, err := WriteObject(objectsDir, data)
	if err != nil {
		t.Fatalf("WriteObject() error = %v", err)
	}

	wantHash := testSHA256Hex(data)
	if hash != wantHash {
		t.Fatalf("unexpected hash: got %q want %q", hash, wantHash)
	}

	objectPath := filepath.Join(objectsDir, wantHash)
	gotData, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatalf("read object file %q: %v", objectPath, err)
	}
	if string(gotData) != string(data) {
		t.Fatalf("object file content mismatch")
	}
}

func TestWriteObjectDeduplicatesWithoutRewritingExistingObject(t *testing.T) {
	objectsDir := t.TempDir()
	data := []byte("same bytes\n")

	hash, err := WriteObject(objectsDir, data)
	if err != nil {
		t.Fatalf("WriteObject(first) error = %v", err)
	}

	objectPath := filepath.Join(objectsDir, hash)
	beforeInfo, err := os.Stat(objectPath)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	hash2, err := WriteObject(objectsDir, data)
	if err != nil {
		t.Fatalf("WriteObject(second) error = %v", err)
	}
	if hash2 != hash {
		t.Fatalf("hash changed on duplicate write: %q vs %q", hash2, hash)
	}

	afterInfo, err := os.Stat(objectPath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}

	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatalf("expected dedupe to avoid rewrite: modtime changed from %v to %v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
}

func TestWriteObjectFailedTempWriteNeverCreatesCommittedObject(t *testing.T) {
	objectsDir := t.TempDir()
	data := []byte("partial-write-data")
	wantHash := testSHA256Hex(data)
	objectPath := filepath.Join(objectsDir, wantHash)

	origWriteAll := writeAllData
	writeAllData = func(f *os.File, b []byte) error {
		half := len(b) / 2
		if half == 0 {
			half = 1
		}
		if _, err := f.Write(b[:half]); err != nil {
			return err
		}
		return os.ErrInvalid
	}
	t.Cleanup(func() { writeAllData = origWriteAll })

	if _, err := WriteObject(objectsDir, data); err == nil {
		t.Fatalf("WriteObject() expected error from injected partial write")
	}

	if _, err := os.Stat(objectPath); !os.IsNotExist(err) {
		t.Fatalf("expected committed object file to be absent, stat err=%v", err)
	}
}

func testSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
