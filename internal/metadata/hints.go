package metadata

import (
	"context"
	"time"
)

// HintKind identifies why a replication hint exists.
type HintKind string

const (
	HintKindPut      HintKind = "put"
	HintKindTombstone HintKind = "tombstone"
	HintKindRepair   HintKind = "repair"
)

// ReplicationHint is durable work to deliver a version to a target node.
type ReplicationHint struct {
	ID             string
	TargetNode     string
	BucketName     string
	Key            string
	Version        int64
	ETag           string
	Kind           HintKind
	ContentType    string
	CustomMetadata map[string]string
	SourceNode     string
	SourcePath     string // pinned blob bytes for put/repair hints (survives overwrites)
	Attempts       int
	NextAttemptAt  time.Time
	Status         string
	LastError      string
	CreatedAt      time.Time
}

// HintStore persists replication and repair hints.
type HintStore interface {
	CreateHint(ctx context.Context, hint ReplicationHint) error
	ListDueHints(ctx context.Context, now time.Time, limit int) ([]ReplicationHint, error)
	MarkHintDelivered(ctx context.Context, id string) error
	RecordHintFailure(ctx context.Context, id string, nextAttempt time.Time, lastError string) error
	CountPendingHints(ctx context.Context) (int64, error)
	ResetHintsForTarget(ctx context.Context, targetNode string) error
}
