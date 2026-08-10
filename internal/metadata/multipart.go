package metadata

import "time"

// MultipartUpload tracks an in-progress multipart upload session.
type MultipartUpload struct {
	ID          string
	BucketName  string
	Key         string
	ContentType string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MultipartPart is a stored upload part.
type MultipartPart struct {
	UploadID    string
	PartNumber  int
	Size        int64
	ETag        string
	StoragePath string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CompletePart identifies a part in a complete request.
type CompletePart struct {
	PartNumber int
	ETag       string
}
