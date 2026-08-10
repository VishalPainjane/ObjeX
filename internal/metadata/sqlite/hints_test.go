package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/VishalPainjane/objex/internal/metadata"
	mdsqlite "github.com/VishalPainjane/objex/internal/metadata/sqlite"
)

func TestHintCRUD(t *testing.T) {
	dir := t.TempDir()
	store, err := mdsqlite.Open(dir + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	hint := metadata.ReplicationHint{
		TargetNode:    "node-3",
		BucketName:    "b",
		Key:           "k",
		Version:       2,
		ETag:          "abc",
		Kind:          metadata.HintKindPut,
		SourceNode:    "node-1",
		NextAttemptAt: now,
		Status:        "pending",
	}
	if err := store.CreateHint(ctx, hint); err != nil {
		t.Fatal(err)
	}

	pending, err := store.CountPendingHints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("pending = %d", pending)
	}

	due, err := store.ListDueHints(ctx, now.Add(time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("due hints = %d", len(due))
	}

	next := now.Add(5 * time.Second)
	if err := store.RecordHintFailure(ctx, due[0].ID, next, "temporary"); err != nil {
		t.Fatal(err)
	}

	dueNow, _ := store.ListDueHints(ctx, now, 10)
	if len(dueNow) != 0 {
		t.Fatalf("hint should not be due yet, got %d", len(dueNow))
	}

	dueLater, _ := store.ListDueHints(ctx, next, 10)
	if len(dueLater) != 1 {
		t.Fatalf("hint should be due after backoff, got %d", len(dueLater))
	}

	if err := store.MarkHintDelivered(ctx, dueLater[0].ID); err != nil {
		t.Fatal(err)
	}
	pending, _ = store.CountPendingHints(ctx)
	if pending != 0 {
		t.Fatalf("pending after delivery = %d", pending)
	}
}

func TestListHintProtectedStoragePaths(t *testing.T) {
	dir := t.TempDir()
	store, err := mdsqlite.Open(dir + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	path := dir + "/blobs/_hints/node-3/b/k/1/key.blob"
	hint := metadata.ReplicationHint{
		TargetNode:    "node-3",
		BucketName:    "b",
		Key:           "k",
		Version:       1,
		ETag:          "abc",
		Kind:          metadata.HintKindPut,
		SourceNode:    "node-1",
		SourcePath:    path,
		NextAttemptAt: time.Now().UTC(),
		Status:        "pending",
	}
	if err := store.CreateHint(ctx, hint); err != nil {
		t.Fatal(err)
	}

	paths, err := store.ListHintProtectedStoragePaths(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != path {
		t.Fatalf("protected paths = %#v", paths)
	}
}
