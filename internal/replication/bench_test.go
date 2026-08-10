package replication_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/VishalPainjane/objex/internal/cluster"
	"github.com/VishalPainjane/objex/internal/metadata/sqlite"
	"github.com/VishalPainjane/objex/internal/object"
	"github.com/VishalPainjane/objex/internal/quorum"
	"github.com/VishalPainjane/objex/internal/replication"
	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

func benchCoordinator(b *testing.B, w int) *replication.Coordinator {
	dir := b.TempDir()
	meta, err := sqlite.Open(dir + "/meta.db")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { meta.Close() })
	blob, err := filesystem.New(dir+"/blobs", nil)
	if err != nil {
		b.Fatal(err)
	}
	nodes := []cluster.Node{
		{ID: "n1", Address: "http://127.0.0.1:1"},
		{ID: "n2", Address: "http://127.0.0.1:2"},
		{ID: "n3", Address: "http://127.0.0.1:3"},
	}
	mem, _ := cluster.NewStaticMembership("n1", nodes)
	placer := cluster.NewRendezvousPlacer(mem, 3, nil)
	svc := object.NewService(meta, blob, 0, 0)
	_ = meta.CreateBucket(context.Background(), "b")
	rep := replication.NewReplicator("token", nil, nil)
	q := quorum.Config{N: 3, W: w, R: 2}
	return replication.NewCoordinator("n1", mem, placer, svc, meta, rep, q, nil)
}

func BenchmarkQuorumPutW2(b *testing.B) {
	coord := benchCoordinator(b, 2)
	ctx := context.Background()
	_ = coord // single-node bench measures coordinator overhead without remote I/O
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 4096))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body.Seek(0, io.SeekStart)
		_, _ = coord.PutObject(ctx, object.PutObjectInput{
			BucketName: "b",
			Key:        "k",
			Body:       body,
		})
	}
}

func BenchmarkQuorumPutW3(b *testing.B) {
	coord := benchCoordinator(b, 3)
	ctx := context.Background()
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 4096))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body.Seek(0, io.SeekStart)
		_, _ = coord.PutObject(ctx, object.PutObjectInput{
			BucketName: "b",
			Key:        "k",
			Body:       body,
		})
	}
}
