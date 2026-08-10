package jobs_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/VishalPainjane/objex/internal/jobs"
	"github.com/VishalPainjane/objex/internal/metadata"
	"github.com/VishalPainjane/objex/internal/metadata/sqlite"
	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

func TestOrphanBlobCleanup(t *testing.T) {
	dir := t.TempDir()
	meta, err := sqlite.Open(dir + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()
	blob, err := filesystem.New(dir+"/blobs", nil)
	if err != nil {
		t.Fatal(err)
	}

	orphanPath := filepath.Join(dir, "blobs", "testbucket", "aa", "bb", "orphan.blob")
	if err := os.MkdirAll(filepath.Dir(orphanPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanup := jobs.NewCleanup(meta, blob, nil)
	deleted, err := cleanup.RunOrphanBlobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if deleted < 1 {
		t.Fatalf("expected orphan deleted, got %d", deleted)
	}
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatal("orphan file still exists")
	}
}

func TestAbandonedMultipartCleanup(t *testing.T) {
	dir := t.TempDir()
	meta, err := sqlite.Open(dir + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()
	blob, err := filesystem.New(dir+"/blobs", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := meta.CreateBucket(ctx, "mp"); err != nil {
		t.Fatal(err)
	}
	uploadID, err := meta.CreateMultipartUpload(ctx, "mp", "key", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}

	_, err = meta.DB().ExecContext(ctx,
		`UPDATE multipart_uploads SET created_at = ? WHERE id = ?`,
		time.Now().Add(-8*24*time.Hour).UTC().Format(time.RFC3339Nano), uploadID,
	)
	if err != nil {
		t.Fatal(err)
	}

	cleanup := jobs.NewCleanup(meta, blob, nil)
	n, err := cleanup.RunAbandonedMultipart(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected upload deleted, got %d", n)
	}
}

func TestOrphanBlobCleanupSkipsHintStagedBlobs(t *testing.T) {
	dir := t.TempDir()
	meta, err := sqlite.Open(dir + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()
	blob, err := filesystem.New(dir+"/blobs", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	staged := filepath.Join(dir, "blobs", "_hints", "node-3", "b", "1", "k.blob")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("hint-stash"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := meta.CreateHint(ctx, metadata.ReplicationHint{
		TargetNode:    "node-3",
		BucketName:    "b",
		Key:           "k",
		Version:       1,
		ETag:          "etag",
		Kind:          metadata.HintKindPut,
		SourceNode:    "node-1",
		SourcePath:    staged,
		NextAttemptAt: time.Now().UTC(),
		Status:        "pending",
	}); err != nil {
		t.Fatal(err)
	}

	cleanup := jobs.NewCleanup(meta, blob, nil)
	deleted, err := cleanup.RunOrphanBlobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("deleted %d hint-staged blobs", deleted)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("hint staged blob removed: %v", err)
	}
}

func TestCleanupDoesNotDeleteKnownBlob(t *testing.T) {
	dir := t.TempDir()
	meta, err := sqlite.Open(dir + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()
	blob, err := filesystem.New(dir+"/blobs", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := meta.CreateBucket(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	put, err := blob.Put(ctx, "b", "obj", bytes.NewReader([]byte("data")))
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.PutObject(ctx, metadata.Object{
		BucketName:  "b",
		Key:         "obj",
		Size:        4,
		ETag:        "abc",
		StoragePath: put.Path,
	}); err != nil {
		t.Fatal(err)
	}

	cleanup := jobs.NewCleanup(meta, blob, nil)
	deleted, err := cleanup.RunOrphanBlobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("deleted %d known blobs", deleted)
	}
}
