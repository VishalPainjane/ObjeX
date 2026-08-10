package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VishalPainjane/objex/internal/metadata"
	"github.com/VishalPainjane/objex/internal/metadata/sqlite"
)

func openTestDB(t *testing.T) *sqlite.Store {
	path := t.TempDir() + "/test.db"
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func TestBucketLifecycle(t *testing.T) {
	s := openTestDB(t)
	defer s.Close()
	ctx := context.Background()

	if err := s.CreateBucket(ctx, "my-bucket"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	ok, err := s.BucketExists(ctx, "my-bucket")
	if err != nil || !ok {
		t.Fatalf("BucketExists: ok=%v err=%v", ok, err)
	}
	if err := s.CreateBucket(ctx, "my-bucket"); !errors.Is(err, metadata.ErrBucketExists) {
		t.Fatalf("duplicate bucket: %v", err)
	}

	buckets, err := s.ListBuckets(ctx)
	if err != nil || len(buckets) != 1 {
		t.Fatalf("ListBuckets: %v len=%d", err, len(buckets))
	}

	if err := s.DeleteBucket(ctx, "my-bucket"); err != nil {
		t.Fatalf("DeleteBucket: %v", err)
	}
	if err := s.DeleteBucket(ctx, "my-bucket"); !errors.Is(err, metadata.ErrBucketNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestObjectPutGetDelete(t *testing.T) {
	s := openTestDB(t)
	defer s.Close()
	ctx := context.Background()
	if err := s.CreateBucket(ctx, "b"); err != nil {
		t.Fatal(err)
	}

	obj := metadata.Object{
		BucketName:  "b",
		Key:         "photos/trip.jpg",
		Size:        100,
		ContentType: "image/jpeg",
		ETag:        "abc123",
		StoragePath: "/data/blobs/x.blob",
		CustomMetadata: map[string]string{"author": "alice"},
	}
	if err := s.PutObject(ctx, obj); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	got, err := s.GetObject(ctx, "b", "photos/trip.jpg")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if got.Size != 100 || got.ETag != "abc123" {
		t.Errorf("got size=%d etag=%s", got.Size, got.ETag)
	}
	if got.CustomMetadata["author"] != "alice" {
		t.Errorf("metadata = %v", got.CustomMetadata)
	}

	b, err := s.GetBucket(ctx, "b")
	if err != nil || b.ObjectCount != 1 || b.TotalSize != 100 {
		t.Fatalf("bucket stats: count=%d size=%d err=%v", b.ObjectCount, b.TotalSize, err)
	}

	// overwrite
	obj.Size = 200
	obj.ETag = "def456"
	if err := s.PutObject(ctx, obj); err != nil {
		t.Fatal(err)
	}
	b, _ = s.GetBucket(ctx, "b")
	if b.ObjectCount != 1 || b.TotalSize != 200 {
		t.Fatalf("after overwrite: count=%d size=%d", b.ObjectCount, b.TotalSize)
	}

	if err := s.DeleteObject(ctx, "b", "photos/trip.jpg"); err != nil {
		t.Fatal(err)
	}
	_, err = s.GetObject(ctx, "b", "photos/trip.jpg")
	if !errors.Is(err, metadata.ErrObjectNotFound) {
		t.Fatalf("expected not found: %v", err)
	}
	if err := s.DeleteObject(ctx, "b", "photos/trip.jpg"); err != nil {
		t.Fatal(err) // idempotent
	}
}

func TestDeleteNonEmptyBucket(t *testing.T) {
	s := openTestDB(t)
	defer s.Close()
	ctx := context.Background()
	s.CreateBucket(ctx, "b")
	s.PutObject(ctx, metadata.Object{BucketName: "b", Key: "k", Size: 1, ETag: "e", StoragePath: "/p"})
	err := s.DeleteBucket(ctx, "b")
	if !errors.Is(err, metadata.ErrBucketNotEmpty) {
		t.Fatalf("expected bucket not empty: %v", err)
	}
}

func TestListObjectsPrefixDelimiter(t *testing.T) {
	s := openTestDB(t)
	defer s.Close()
	ctx := context.Background()
	s.CreateBucket(ctx, "b")
	for _, key := range []string{"photos/a.jpg", "photos/b.jpg", "docs/readme.txt"} {
		s.PutObject(ctx, metadata.Object{BucketName: "b", Key: key, Size: 1, ETag: "e", StoragePath: "/p"})
	}

	res, err := s.ListObjects(ctx, "b", metadata.ListOptions{Prefix: "photos/", Delimiter: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Objects) != 2 {
		t.Errorf("objects = %d, want 2", len(res.Objects))
	}
}

func TestListPagination(t *testing.T) {
	s := openTestDB(t)
	defer s.Close()
	ctx := context.Background()
	s.CreateBucket(ctx, "b")
	for i := 0; i < 5; i++ {
		key := "obj-" + string(rune('a'+i))
		s.PutObject(ctx, metadata.Object{BucketName: "b", Key: key, Size: 1, ETag: "e", StoragePath: "/p"})
	}

	res, err := s.ListObjects(ctx, "b", metadata.ListOptions{MaxKeys: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Objects) != 2 || !res.IsTruncated {
		t.Fatalf("page1: len=%d truncated=%v", len(res.Objects), res.IsTruncated)
	}

	res2, err := s.ListObjects(ctx, "b", metadata.ListOptions{MaxKeys: 2, ContinuationToken: res.NextContinuationToken})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Objects) != 2 {
		t.Fatalf("page2 len=%d", len(res2.Objects))
	}
}

func TestPutObjectRequiresBucket(t *testing.T) {
	s := openTestDB(t)
	defer s.Close()
	err := s.PutObject(context.Background(), metadata.Object{BucketName: "missing", Key: "k", Size: 1, ETag: "e", StoragePath: "/p"})
	if !errors.Is(err, metadata.ErrBucketNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestTimestampsStoredUTC(t *testing.T) {
	s := openTestDB(t)
	defer s.Close()
	ctx := context.Background()
	s.CreateBucket(ctx, "b")
	before := time.Now().UTC()
	s.PutObject(ctx, metadata.Object{BucketName: "b", Key: "k", Size: 1, ETag: "e", StoragePath: "/p"})
	o, _ := s.GetObject(ctx, "b", "k")
	if o.CreatedAt.Before(before.Add(-time.Second)) {
		t.Errorf("created_at too old: %v", o.CreatedAt)
	}
}
