package object

import (
	"bytes"
	"context"
	"testing"

	"github.com/VishalPainjane/objex/internal/metadata/sqlite"
	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

func TestPutObjectMaxUploadBytes(t *testing.T) {
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

	svc := NewService(meta, blob, 100, 0)
	ctx := context.Background()
	if err := svc.CreateBucket(ctx, "test-bucket"); err != nil {
		t.Fatal(err)
	}

	large := bytes.Repeat([]byte("x"), 101)
	_, err = svc.PutObject(ctx, PutObjectInput{
		BucketName: "test-bucket",
		Key:        "big.bin",
		Body:       bytes.NewReader(large),
	})
	oe, ok := AsError(err)
	if !ok || oe.Code != CodeEntityTooLarge {
		t.Fatalf("expected EntityTooLarge, got %v", err)
	}
	exists, err := blob.Exists(ctx, "test-bucket", "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("oversize upload should not leave blob")
	}
}
