package object_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/VishalPainjane/objex/internal/metadata/sqlite"
	"github.com/VishalPainjane/objex/internal/object"
	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

func TestLargeObjectStreaming(t *testing.T) {
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
	svc := object.NewService(meta, blob, 0, 0)
	ctx := context.Background()

	if err := svc.CreateBucket(ctx, "big"); err != nil {
		t.Fatal(err)
	}

	size := 10 * 1024 * 1024 // 10 MB
	data := bytes.Repeat([]byte("x"), size)
	_, err = svc.PutObject(ctx, object.PutObjectInput{
		BucketName: "big",
		Key:        "large.bin",
		Body:       bytes.NewReader(data),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetObject(ctx, "big", "large.bin", false)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	n, err := io.Copy(io.Discard, result.Body)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(size) {
		t.Fatalf("size = %d want %d", n, size)
	}
}
