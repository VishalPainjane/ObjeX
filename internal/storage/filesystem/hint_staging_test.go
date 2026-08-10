package filesystem_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

func TestStageHintCopyAndOpenPath(t *testing.T) {
	dir := t.TempDir()
	store, err := filesystem.New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, "photos", "aa", "bb", "obj.blob")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("pinned-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, size, err := store.StageHintCopy(src, "node-3", "photos", "2024/a.jpg", 7)
	if err != nil {
		t.Fatal(err)
	}
	if size != 12 {
		t.Fatalf("size = %d", size)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("staged file missing: %v", err)
	}

	if err := os.WriteFile(src, []byte("overwritten"), 0o644); err != nil {
		t.Fatal(err)
	}

	rc, gotSize, err := store.OpenPath(staged)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, _ := rc.Read(buf)
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	if gotSize != 12 || string(buf[:n]) != "pinned-bytes" {
		t.Fatalf("staged content = %q size=%d", buf[:n], gotSize)
	}

	if err := store.RemovePath(staged); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatal("expected staged file removed")
	}
}
