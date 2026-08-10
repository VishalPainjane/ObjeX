package metadata

import (
	"context"
	"errors"
	"time"
)

var (
	ErrBucketNotFound  = errors.New("bucket not found")
	ErrBucketExists    = errors.New("bucket already exists")
	ErrBucketNotEmpty  = errors.New("bucket not empty")
	ErrObjectNotFound  = errors.New("object not found")
	ErrNoSuchUpload    = errors.New("no such upload")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrStaleReplica    = errors.New("stale replica write")
)

// Bucket metadata.
type Bucket struct {
	ID          string
	Name        string
	ObjectCount int64
	TotalSize   int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Object metadata for a stored blob.
type Object struct {
	ID                 string
	BucketName         string
	Key                string
	Size               int64
	ContentType        string
	ETag               string
	StoragePath        string
	CustomMetadata     map[string]string
	Version            int64
	ReplicationStatus  string // primary only: "replicated" or "partial"
	Deleted            bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ListOptions filters object listing.
type ListOptions struct {
	Prefix            string
	Delimiter         string
	MaxKeys           int
	ContinuationToken string
	StartAfter        string
}

// ListResult holds listed objects and virtual folder prefixes.
type ListResult struct {
	Objects              []Object
	CommonPrefixes       []string
	NextContinuationToken string
	IsTruncated          bool
}

// Store abstracts object and bucket metadata persistence.
type Store interface {
	CreateBucket(ctx context.Context, name string) error
	DeleteBucket(ctx context.Context, name string) error
	BucketExists(ctx context.Context, name string) (bool, error)
	GetBucket(ctx context.Context, name string) (*Bucket, error)
	ListBuckets(ctx context.Context) ([]Bucket, error)

	PutObject(ctx context.Context, obj Object) error
	PutTombstone(ctx context.Context, bucket, key string, version int64) error
	UpdateReplicationStatus(ctx context.Context, bucket, key, status string) error
	GetObject(ctx context.Context, bucket, key string) (*Object, error)
	DeleteObject(ctx context.Context, bucket, key string) error
	ListObjects(ctx context.Context, bucket string, opts ListOptions) (ListResult, error)

	CreateHint(ctx context.Context, hint ReplicationHint) error
	ListDueHints(ctx context.Context, now time.Time, limit int) ([]ReplicationHint, error)
	MarkHintDelivered(ctx context.Context, id string) error
	RecordHintFailure(ctx context.Context, id string, nextAttempt time.Time, lastError string) error
	CountPendingHints(ctx context.Context) (int64, error)
	ListHintProtectedStoragePaths(ctx context.Context) ([]string, error)
	ResetHintsForTarget(ctx context.Context, targetNode string) error

	ListObjectsByReplicationStatus(ctx context.Context, status string, limit int) ([]Object, error)

	CreateMultipartUpload(ctx context.Context, bucket, key, contentType string) (string, error)
	GetMultipartUpload(ctx context.Context, uploadID, bucket, key string) (*MultipartUpload, error)
	ListMultipartUploads(ctx context.Context, bucket string) ([]MultipartUpload, error)
	DeleteMultipartUpload(ctx context.Context, uploadID string) error
	UpsertMultipartPart(ctx context.Context, part MultipartPart) error
	ListMultipartParts(ctx context.Context, uploadID string) ([]MultipartPart, error)
	DeleteAbandonedMultipartUploads(ctx context.Context, olderThan time.Time) (int, error)
	AllObjectStoragePaths(ctx context.Context) ([]string, error)
	ListAllStoredObjects(ctx context.Context) ([]Object, error)
}
