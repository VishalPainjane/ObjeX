package object

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/VishalPainjane/objex/internal/metadata/sqlite"
	"github.com/VishalPainjane/objex/internal/storage"
	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

func newTestService(t *testing.T) *Service {
	dir := t.TempDir()
	meta, err := sqlite.Open(dir + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { meta.Close() })
	blob, err := filesystem.New(dir+"/blobs", nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(meta, blob, 0, 0)
}

func TestPutGetDeleteRoundTrip(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	s.CreateBucket(ctx, "test-bucket")

	body := []byte("Hello, ObjeX round-trip test!")
	etag, err := s.PutObject(ctx, PutObjectInput{
		BucketName:  "test-bucket",
		Key:         "lifecycle-test.txt",
		ContentType: "text/plain",
		Body:        bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if etag == "" {
		t.Fatal("empty etag")
	}

	got, err := s.GetObject(ctx, "test-bucket", "lifecycle-test.txt", false)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	data, _ := io.ReadAll(got.Body)
	got.Body.Close()
	if !bytes.Equal(data, body) {
		t.Fatalf("body mismatch")
	}
	if got.ETag != etag {
		t.Errorf("etag mismatch")
	}

	if err := s.DeleteObject(ctx, "test-bucket", "lifecycle-test.txt"); err != nil {
		t.Fatal(err)
	}
	_, err = s.GetObject(ctx, "test-bucket", "lifecycle-test.txt", false)
	if oe, ok := AsError(err); !ok || oe.Code != CodeNoSuchKey {
		t.Fatalf("expected NoSuchKey: %v", err)
	}
}

func TestPutOrphanCleanupOnMetadataFailure(t *testing.T) {
	dir := t.TempDir()
	meta, _ := sqlite.Open(dir + "/meta.db")
	t.Cleanup(func() { meta.Close() })
	blob, _ := filesystem.New(dir+"/blobs", nil)
	s := NewService(meta, blob, 0, 0)
	ctx := context.Background()

	_, err := s.PutObject(ctx, PutObjectInput{
		BucketName: "missing",
		Key:        "k",
		Body:       bytes.NewReader([]byte("x")),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	ok, _ := blob.Exists(ctx, "missing", "k")
	if ok {
		t.Fatal("orphan blob should be deleted")
	}
}

func TestDeleteNonexistentObjectIdempotent(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	s.CreateBucket(ctx, "b")
	if err := s.DeleteObject(ctx, "b", "missing"); err != nil {
		t.Fatal(err)
	}
}

func TestBucketLifecycle(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	if err := s.CreateBucket(ctx, "my-bucket"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBucket(ctx, "my-bucket"); err == nil {
		t.Fatal("duplicate bucket")
	}
	if err := s.HeadBucket(ctx, "my-bucket"); err != nil {
		t.Fatal(err)
	}
	buckets, err := s.ListBuckets(ctx)
	if err != nil || len(buckets) != 1 {
		t.Fatalf("ListBuckets: %v len=%d", err, len(buckets))
	}
	if err := s.DeleteBucket(ctx, "my-bucket"); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrityVerify(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	if err := s.CreateBucket(ctx, "bbb"); err != nil {
		t.Fatal(err)
	}
	_, err := s.PutObject(ctx, PutObjectInput{BucketName: "bbb", Key: "k", Body: bytes.NewReader([]byte("ok"))})
	if err != nil {
		t.Fatal(err)
	}

	fsStore, ok := s.blob.(*filesystem.Storage)
	if !ok {
		t.Fatal("expected filesystem storage backend")
	}
	path, err := storage.ResolveBlobPath(fsStore.BasePath(), "bbb", "k")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = s.GetObject(ctx, "bbb", "k", true)
	oe, ok := AsError(err)
	if !ok || oe.Code != CodeInternalError {
		t.Fatalf("expected integrity error: %v", err)
	}
}
