package object

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/VishalPainjane/objex/internal/cluster"
	"github.com/VishalPainjane/objex/internal/hash"
	"github.com/VishalPainjane/objex/internal/metadata"
	"github.com/VishalPainjane/objex/internal/metrics"
	"github.com/VishalPainjane/objex/internal/storage"
	"github.com/VishalPainjane/objex/internal/validation"
)

// Service orchestrates metadata and blob storage for object operations.
type Service struct {
	meta           metadata.Store
	blob           storage.Storage
	maxUploadBytes int64
	minFreeBytes   int64
	placement      cluster.Placer
	logger         *slog.Logger
}

// NewService constructs an object service.
func NewService(meta metadata.Store, blob storage.Storage, maxUploadBytes, minFreeBytes int64) *Service {
	return &Service{
		meta:           meta,
		blob:           blob,
		maxUploadBytes: maxUploadBytes,
		minFreeBytes:   minFreeBytes,
	}
}

// SetPlacement wires deterministic placement for observability (no remote routing in Phase 1).
func (s *Service) SetPlacement(p cluster.Placer, logger *slog.Logger) {
	s.placement = p
	s.logger = logger
}

func (s *Service) recordPlacement(bucket, key, operation string) {
	if s.placement == nil {
		return
	}
	metrics.RecordPlacement(operation)
	result, err := s.placement.Locate(bucket, key)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("placement locate failed", "bucket", bucket, "key", key, "operation", operation, "error", err)
		}
		return
	}
	if s.logger != nil {
		s.logger.Debug("object placement",
			"bucket", bucket,
			"key", key,
			"primary_node", result.Primary.ID,
			"operation", operation,
		)
	}
}

const minPartSize = 5 * 1024 * 1024 // 5 MiB — S3 minimum except last part

// CreateBucket creates a new bucket.
func (s *Service) CreateBucket(ctx context.Context, name string) error {
	if msg := validation.BucketNameError(name); msg != "" {
		return newErr(CodeInvalidBucketName, msg, 400)
	}
	err := s.meta.CreateBucket(ctx, name)
	if errors.Is(err, metadata.ErrBucketExists) {
		return newErr(CodeBucketAlreadyExists, "The bucket '"+name+"' already exists.", 409)
	}
	return err
}

// DeleteBucket deletes an empty bucket.
func (s *Service) DeleteBucket(ctx context.Context, name string) error {
	err := s.meta.DeleteBucket(ctx, name)
	if errors.Is(err, metadata.ErrBucketNotFound) {
		return newErr(CodeNoSuchBucket, "The specified bucket does not exist.", 404)
	}
	if errors.Is(err, metadata.ErrBucketNotEmpty) {
		return newErr(CodeBucketNotEmpty, "The bucket you tried to delete is not empty.", 409)
	}
	return err
}

// HeadBucket checks bucket existence.
func (s *Service) HeadBucket(ctx context.Context, name string) error {
	ok, err := s.meta.BucketExists(ctx, name)
	if err != nil {
		return err
	}
	if !ok {
		return newErr(CodeNoSuchBucket, "The specified bucket does not exist.", 404)
	}
	return nil
}

// ListBuckets returns all buckets.
func (s *Service) ListBuckets(ctx context.Context) ([]metadata.Bucket, error) {
	return s.meta.ListBuckets(ctx)
}

// PutObjectInput describes an upload.
type PutObjectInput struct {
	BucketName     string
	Key            string
	ContentType    string
	CustomMetadata map[string]string
	Body           io.Reader
}

// PutObject stores an object, streaming the body and computing MD5 ETag.
func (s *Service) PutObject(ctx context.Context, in PutObjectInput) (string, error) {
	version, err := s.NextObjectVersion(ctx, in.BucketName, in.Key)
	if err != nil {
		return "", err
	}
	return s.PutObjectWithVersion(ctx, in, version, "replicated")
}

// NextObjectVersion returns the version to assign on the next write.
func (s *Service) NextObjectVersion(ctx context.Context, bucket, key string) (int64, error) {
	existing, err := s.meta.GetObject(ctx, bucket, key)
	if errors.Is(err, metadata.ErrObjectNotFound) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return existing.Version + 1, nil
}

// PutObjectWithVersion stores an object at a specific generation (primary or replica).
func (s *Service) PutObjectWithVersion(ctx context.Context, in PutObjectInput, version int64, replicationStatus string) (string, error) {
	if msg := validation.KeyError(in.Key); msg != "" {
		return "", newErr(CodeInvalidArgument, msg, 400)
	}
	s.recordPlacement(in.BucketName, in.Key, "put_object")
	exists, err := s.meta.BucketExists(ctx, in.BucketName)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", newErr(CodeNoSuchBucket, "The specified bucket does not exist.", 404)
	}
	if s.minFreeBytes > 0 && s.blob.AvailableFreeBytes() < s.minFreeBytes {
		return "", newErr(CodeEntityTooLarge, "Insufficient disk space.", 507)
	}

	body := in.Body
	if s.maxUploadBytes > 0 {
		body = newLimitReader(in.Body, s.maxUploadBytes)
	}

	hasher := hash.NewMD5Reader(body)
	putResult, err := s.blob.Put(ctx, in.BucketName, in.Key, hasher)
	if err != nil {
		if errors.Is(err, errUploadTooLarge) {
			return "", newErr(CodeEntityTooLarge, "Object exceeds maximum upload size.", 400)
		}
		return "", err
	}
	if herr := hasher.Err(); herr != nil {
		if errors.Is(herr, errUploadTooLarge) {
			s.blob.Delete(ctx, in.BucketName, in.Key)
			return "", newErr(CodeEntityTooLarge, "Object exceeds maximum upload size.", 400)
		}
		if herr != io.EOF {
			s.blob.Delete(ctx, in.BucketName, in.Key)
			return "", herr
		}
	}

	etag := hasher.ETag()
	contentType := in.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	obj := metadata.Object{
		BucketName:        in.BucketName,
		Key:               in.Key,
		Size:              putResult.Size,
		ContentType:       contentType,
		ETag:              etag,
		StoragePath:       putResult.Path,
		CustomMetadata:    in.CustomMetadata,
		Version:           version,
		ReplicationStatus: replicationStatus,
	}
	if err := s.meta.PutObject(ctx, obj); err != nil {
		// Metadata commit failed — remove orphan blob.
		s.blob.Delete(ctx, in.BucketName, in.Key)
		return "", err
	}
	return etag, nil
}

// PutReplicaInput describes an internal replica write (no further replication).
type PutReplicaInput struct {
	BucketName     string
	Key            string
	Version        int64
	ExpectedETag   string
	ContentType    string
	CustomMetadata map[string]string
	Body           io.Reader
}

// PutReplica stores a replica locally with version and checksum validation.
func (s *Service) PutReplica(ctx context.Context, in PutReplicaInput) error {
	if msg := validation.KeyError(in.Key); msg != "" {
		return newErr(CodeInvalidArgument, msg, 400)
	}
	exists, err := s.meta.BucketExists(ctx, in.BucketName)
	if err != nil {
		return err
	}
	if !exists {
		return newErr(CodeNoSuchBucket, "The specified bucket does not exist.", 404)
	}

	existing, err := s.meta.GetObject(ctx, in.BucketName, in.Key)
	if err != nil && !errors.Is(err, metadata.ErrObjectNotFound) {
		return err
	}
	if existing != nil {
		if in.Version < existing.Version {
			return newErr(CodeInvalidArgument, "Stale replica write rejected.", 409)
		}
		if in.Version == existing.Version {
			if existing.Deleted {
				return newErr(CodeInvalidArgument, "Stale replica write rejected.", 409)
			}
			if existing.ETag == in.ExpectedETag {
				return nil
			}
			return newErr(CodeInvalidArgument, "Version conflict: same version with different ETag.", 409)
		}
	}

	body := in.Body
	if s.maxUploadBytes > 0 {
		body = newLimitReader(in.Body, s.maxUploadBytes)
	}

	hasher := hash.NewMD5Reader(body)
	putResult, err := s.blob.Put(ctx, in.BucketName, in.Key, hasher)
	if err != nil {
		return err
	}
	if herr := hasher.Err(); herr != nil && herr != io.EOF {
		s.blob.Delete(ctx, in.BucketName, in.Key)
		return herr
	}
	etag := hasher.ETag()
	if etag != in.ExpectedETag {
		s.blob.Delete(ctx, in.BucketName, in.Key)
		return newErr(CodeInvalidArgument, "Replica checksum mismatch.", 400)
	}

	contentType := in.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	obj := metadata.Object{
		BucketName:        in.BucketName,
		Key:               in.Key,
		Size:              putResult.Size,
		ContentType:       contentType,
		ETag:              etag,
		StoragePath:       putResult.Path,
		CustomMetadata:    in.CustomMetadata,
		Version:           in.Version,
		ReplicationStatus: "replicated",
	}
	if err := s.meta.PutObject(ctx, obj); err != nil {
		s.blob.Delete(ctx, in.BucketName, in.Key)
		return err
	}
	return nil
}

// SetReplicationStatus updates primary replication status metadata.
func (s *Service) SetReplicationStatus(ctx context.Context, bucket, key, status string) error {
	return s.meta.UpdateReplicationStatus(ctx, bucket, key, status)
}

// GetObjectMetadata returns object metadata without opening the blob.
func (s *Service) GetObjectMetadata(ctx context.Context, bucket, key string) (*metadata.Object, error) {
	obj, err := s.meta.GetObject(ctx, bucket, key)
	if errors.Is(err, metadata.ErrObjectNotFound) {
		return nil, newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
	}
	return obj, err
}

// OpenStoredObject opens the local blob for reading (used for replication streaming).
func (s *Service) OpenStoredObject(ctx context.Context, bucket, key string) (io.ReadCloser, int64, error) {
	rc, info, err := s.blob.Get(ctx, bucket, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, 0, newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
		}
		return nil, 0, err
	}
	return rc, info.Size, nil
}

// StageHintPayload copies the current object bytes to a durable hint stash path.
func (s *Service) StageHintPayload(ctx context.Context, bucket, key string, version int64, targetNode string) (string, int64, error) {
	obj, err := s.meta.GetObject(ctx, bucket, key)
	if err != nil {
		return "", 0, err
	}
	if obj.Deleted || obj.StoragePath == "" {
		return "", 0, fmt.Errorf("no object bytes to stage")
	}
	if st, ok := s.blob.(interface {
		StageHintCopy(srcPath, targetNode, bucket, key string, version int64) (string, int64, error)
	}); ok {
		return st.StageHintCopy(obj.StoragePath, targetNode, bucket, key, version)
	}
	return obj.StoragePath, obj.Size, nil
}

// OpenStoragePath opens a pinned absolute blob path (hint payload).
func (s *Service) OpenStoragePath(path string) (io.ReadCloser, int64, error) {
	if st, ok := s.blob.(interface {
		OpenPath(path string) (io.ReadCloser, int64, error)
	}); ok {
		return st.OpenPath(path)
	}
	return nil, 0, fmt.Errorf("storage backend does not support OpenPath")
}

// RemoveStoragePath removes a pinned blob path after hint delivery.
func (s *Service) RemoveStoragePath(path string) error {
	if st, ok := s.blob.(interface {
		RemovePath(path string) error
	}); ok {
		return st.RemovePath(path)
	}
	return nil
}

// GetObjectResult is returned from GetObject.
type GetObjectResult struct {
	Body           io.ReadCloser
	Size           int64
	ContentType    string
	ETag           string
	LastModified   time.Time
	CustomMetadata map[string]string
}

// GetObject opens an object for reading.
func (s *Service) GetObject(ctx context.Context, bucket, key string, verifyIntegrity bool) (*GetObjectResult, error) {
	if msg := validation.KeyError(key); msg != "" {
		return nil, newErr(CodeInvalidArgument, msg, 400)
	}
	s.recordPlacement(bucket, key, "get_object")
	if ok, err := s.meta.BucketExists(ctx, bucket); err != nil {
		return nil, err
	} else if !ok {
		return nil, newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
	}

	obj, err := s.meta.GetObject(ctx, bucket, key)
	if errors.Is(err, metadata.ErrObjectNotFound) {
		return nil, newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
	}
	if err != nil {
		return nil, err
	}
	if obj.Deleted {
		return nil, newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
	}

	rc, info, err := s.blob.Get(ctx, bucket, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
		}
		return nil, err
	}

	if verifyIntegrity {
		hasher := hash.NewMD5Reader(rc)
		if _, err := io.Copy(io.Discard, hasher); err != nil {
			rc.Close()
			return nil, err
		}
		computed := hasher.ETag()
		rc.Close()
		if computed != obj.ETag {
			return nil, newErr(CodeInternalError,
				"Integrity check failed: stored ETag "+obj.ETag+" does not match computed "+computed+".", 500)
		}
		rc, info, err = s.blob.Get(ctx, bucket, key)
		if err != nil {
			return nil, err
		}
	}

	return &GetObjectResult{
		Body:           rc,
		Size:           info.Size,
		ContentType:    obj.ContentType,
		ETag:           obj.ETag,
		LastModified:   obj.UpdatedAt,
		CustomMetadata: obj.CustomMetadata,
	}, nil
}

// HeadObject returns object metadata without body.
func (s *Service) HeadObject(ctx context.Context, bucket, key string) (*metadata.Object, error) {
	if msg := validation.KeyError(key); msg != "" {
		return nil, newErr(CodeInvalidArgument, msg, 400)
	}
	s.recordPlacement(bucket, key, "head_object")
	if ok, err := s.meta.BucketExists(ctx, bucket); err != nil {
		return nil, err
	} else if !ok {
		return nil, newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
	}
	obj, err := s.meta.GetObject(ctx, bucket, key)
	if errors.Is(err, metadata.ErrObjectNotFound) {
		return nil, newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
	}
	if err != nil {
		return nil, err
	}
	if obj.Deleted {
		return nil, newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
	}
	return obj, err
}

// PutTombstone records a versioned delete marker on the primary.
func (s *Service) PutTombstone(ctx context.Context, bucket, key string, version int64) error {
	s.recordPlacement(bucket, key, "delete_object")
	if ok, err := s.meta.BucketExists(ctx, bucket); err != nil {
		return err
	} else if !ok {
		return nil
	}
	existing, err := s.meta.GetObject(ctx, bucket, key)
	if err != nil && !errors.Is(err, metadata.ErrObjectNotFound) {
		return err
	}
	if existing != nil && !existing.Deleted {
		_ = s.blob.Delete(ctx, bucket, key)
	}
	return s.meta.PutTombstone(ctx, bucket, key, version)
}

// PutTombstoneReplica applies a versioned tombstone on a replica node.
func (s *Service) PutTombstoneReplica(ctx context.Context, bucket, key string, version int64) error {
	existing, err := s.meta.GetObject(ctx, bucket, key)
	if err != nil && !errors.Is(err, metadata.ErrObjectNotFound) {
		return err
	}
	if existing != nil {
		if version < existing.Version {
			return newErr(CodeInvalidArgument, "Stale replica delete rejected.", 409)
		}
		if version == existing.Version && existing.Deleted {
			return nil
		}
		if version == existing.Version && !existing.Deleted {
			return newErr(CodeInvalidArgument, "Version conflict on tombstone.", 409)
		}
	}
	if existing != nil && !existing.Deleted {
		_ = s.blob.Delete(ctx, bucket, key)
	}
	return s.meta.PutTombstone(ctx, bucket, key, version)
}

// LocalReplicaState returns metadata for quorum reads on this node.
func (s *Service) LocalReplicaState(ctx context.Context, bucket, key string) (found bool, obj *metadata.Object, err error) {
	obj, err = s.meta.GetObject(ctx, bucket, key)
	if errors.Is(err, metadata.ErrObjectNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return true, obj, nil
}

// DeleteObject removes an object (idempotent).
func (s *Service) DeleteObject(ctx context.Context, bucket, key string) error {
	s.recordPlacement(bucket, key, "delete_object")
	if ok, err := s.meta.BucketExists(ctx, bucket); err != nil {
		return err
	} else if !ok {
		return nil // S3: non-existent bucket → 204
	}

	_, err := s.meta.GetObject(ctx, bucket, key)
	if errors.Is(err, metadata.ErrObjectNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	// Delete metadata before blob so we never leave metadata pointing at a removed file.
	if err := s.meta.DeleteObject(ctx, bucket, key); err != nil {
		return err
	}
	return s.blob.Delete(ctx, bucket, key)
}

// ListObjects lists objects in a bucket.
func (s *Service) ListObjects(ctx context.Context, bucket string, opts metadata.ListOptions) (metadata.ListResult, error) {
	if ok, err := s.meta.BucketExists(ctx, bucket); err != nil {
		return metadata.ListResult{}, err
	} else if !ok {
		return metadata.ListResult{}, newErr(CodeNoSuchBucket, "The specified bucket does not exist.", 404)
	}
	return s.meta.ListObjects(ctx, bucket, opts)
}

