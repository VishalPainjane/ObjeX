package storage

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound indicates the object blob does not exist on storage.
var ErrNotFound = errors.New("object not found")

// ErrPathEscape indicates a resolved path would escape the storage root.
var ErrPathEscape = errors.New("path escapes storage root")

// Info describes a stored blob without opening it for read.
type Info struct {
	Path string
	Size int64
}

// PutResult is returned after a successful write.
type PutResult struct {
	Path string
	Size int64
}

// Storage abstracts physical blob placement. Implementations must support streaming I/O.
type Storage interface {
	Put(ctx context.Context, bucket, key string, body io.Reader) (PutResult, error)
	Get(ctx context.Context, bucket, key string) (io.ReadCloser, Info, error)
	Delete(ctx context.Context, bucket, key string) error
	Stat(ctx context.Context, bucket, key string) (Info, error)
	Exists(ctx context.Context, bucket, key string) (bool, error)
	AvailableFreeBytes() int64

	PutPart(ctx context.Context, uploadID string, partNumber int, body io.Reader) (PartPutResult, error)
	AssembleParts(ctx context.Context, bucket, key string, orderedPartPaths []string) (PutResult, error)
	DeleteUploadParts(ctx context.Context, uploadID string) error
}

// PartPutResult is returned after storing a multipart part.
type PartPutResult struct {
	Path string
	Size int64
	ETag string
}
