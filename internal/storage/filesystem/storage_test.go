package filesystem_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VishalPainjane/objex/internal/storage"
	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

func newTestStorage(t *testing.T) *filesystem.Storage {
	dir := t.TempDir()
	s, err := filesystem.New(dir, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestPutGetRoundTrip(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	body := bytes.Repeat([]byte("abc"), 1024)

	result, err := s.Put(ctx, "photos", "2024/trip.jpg", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if result.Size != int64(len(body)) {
		t.Errorf("Size = %d, want %d", result.Size, len(body))
	}

	rc, info, err := s.Get(ctx, "photos", "2024/trip.jpg")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("body mismatch, len %d vs %d", len(got), len(body))
	}
	if info.Size != int64(len(body)) {
		t.Errorf("info.Size = %d", info.Size)
	}
}

func TestPutCreatesHashedPath(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	_, err := s.Put(ctx, "my-bucket", "folder/file.txt", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	rel := storage.BlobRelativePath("my-bucket", "folder/file.txt")
	full := filepath.Join(s.BasePath(), rel)
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("expected blob at %s: %v", full, err)
	}
}

func TestAtomicWriteNoPartialFinalFile(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	final, err := storage.ResolveBlobPath(s.BasePath(), "bucket", "key")
	if err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte("partial"))
		pw.CloseWithError(errors.New("simulated failure"))
	}()

	_, err = s.Put(ctx, "bucket", "key", pr)
	if err == nil {
		t.Fatal("expected Put error")
	}
	if _, err := os.Stat(final); err == nil {
		t.Fatal("final blob should not exist after failed write")
	}

	tmpCount := 0
	_ = filepath.Walk(filepath.Dir(final), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".tmp") {
			tmpCount++
		}
		return nil
	})
	if tmpCount > 0 {
		t.Errorf("expected no leftover tmp files, found %d", tmpCount)
	}
}

func TestStaleTmpCleanupOnStartup(t *testing.T) {
	dir := t.TempDir()
	blobDir := filepath.Join(dir, "bucket", "aa", "bb")
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stalePath := filepath.Join(blobDir, "deadbeef.blob.abc123.tmp")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stalePath, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := filesystem.New(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stalePath); err == nil {
		t.Fatal("stale tmp file should be removed on startup")
	}
}

func TestDeleteAndExists(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	_, err := s.Put(ctx, "b", "k", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	ok, err := s.Exists(ctx, "b", "k")
	if err != nil || !ok {
		t.Fatalf("Exists: ok=%v err=%v", ok, err)
	}
	if err := s.Delete(ctx, "b", "k"); err != nil {
		t.Fatal(err)
	}
	ok, err = s.Exists(ctx, "b", "k")
	if err != nil || ok {
		t.Fatalf("after delete Exists: ok=%v err=%v", ok, err)
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStorage(t)
	_, _, err := s.Get(context.Background(), "b", "missing")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Get err = %v, want ErrNotFound", err)
	}
}

func TestPathTraversalRejected(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Keys with .. are sanitized; traversal via bucket name should fail path check.
	_, err := s.Put(ctx, "../escape", "key", strings.NewReader("x"))
	if err == nil {
		t.Fatal("expected error for escaping bucket path")
	}
}

func TestOverwriteObject(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	_, err := s.Put(ctx, "b", "k", strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Put(ctx, "b", "k", strings.NewReader("new-data"))
	if err != nil {
		t.Fatal(err)
	}
	rc, _, err := s.Get(ctx, "b", "k")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "new-data" {
		t.Errorf("got %q", got)
	}
}

func TestStreamingLargeObject(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	size := 5 * 1024 * 1024 // 5 MiB
	body := bytes.Repeat([]byte{0xAB}, size)

	_, err := s.Put(ctx, "big", "obj", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, info, err := s.Get(ctx, "big", "obj")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	if info.Size != int64(size) {
		t.Errorf("Size = %d", info.Size)
	}

	n, err := io.Copy(io.Discard, rc)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(size) {
		t.Errorf("read %d bytes, want %d", n, size)
	}
}

func TestStat(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()
	data := "hello"
	_, err := s.Put(ctx, "b", "k", strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Stat(ctx, "b", "k")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != int64(len(data)) {
		t.Errorf("Size = %d", info.Size)
	}
}
