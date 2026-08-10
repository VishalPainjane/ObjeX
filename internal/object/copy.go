package object

import (
	"context"
	"errors"
	"io"

	"github.com/VishalPainjane/objex/internal/hash"
	"github.com/VishalPainjane/objex/internal/metadata"
	"github.com/VishalPainjane/objex/internal/storage"
	"github.com/VishalPainjane/objex/internal/validation"
)

// CopyObject copies an object from src to dst within storage.
func (s *Service) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) (string, error) {
	if msg := validation.KeyError(dstKey); msg != "" {
		return "", newErr(CodeInvalidArgument, msg, 400)
	}
	if msg := validation.KeyError(srcKey); msg != "" {
		return "", newErr(CodeInvalidArgument, msg, 400)
	}

	srcExists, err := s.meta.BucketExists(ctx, srcBucket)
	if err != nil {
		return "", err
	}
	if !srcExists {
		return "", newErr(CodeNoSuchBucket, "Source bucket '"+srcBucket+"' does not exist.", 404)
	}

	srcObj, err := s.meta.GetObject(ctx, srcBucket, srcKey)
	if errors.Is(err, metadata.ErrObjectNotFound) {
		return "", newErr(CodeNoSuchKey, "The specified source key does not exist.", 404)
	}
	if err != nil {
		return "", err
	}

	dstExists, err := s.meta.BucketExists(ctx, dstBucket)
	if err != nil {
		return "", err
	}
	if !dstExists {
		return "", newErr(CodeNoSuchBucket, "The destination bucket does not exist.", 404)
	}

	if s.minFreeBytes > 0 && s.blob.AvailableFreeBytes() < s.minFreeBytes {
		return "", newErr(CodeEntityTooLarge, "Insufficient disk space.", 507)
	}

	rc, _, err := s.blob.Get(ctx, srcBucket, srcKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return "", newErr(CodeNoSuchKey, "The specified source key does not exist.", 404)
		}
		return "", err
	}
	defer rc.Close()

	hasher := hash.NewMD5Reader(rc)
	putResult, err := s.blob.Put(ctx, dstBucket, dstKey, hasher)
	if err != nil {
		return "", err
	}
	if herr := hasher.Err(); herr != nil && herr != io.EOF {
		s.blob.Delete(ctx, dstBucket, dstKey)
		return "", herr
	}

	etag := hasher.ETag()
	obj := metadata.Object{
		BucketName:     dstBucket,
		Key:            dstKey,
		Size:           putResult.Size,
		ContentType:    srcObj.ContentType,
		ETag:           etag,
		StoragePath:    putResult.Path,
		CustomMetadata: srcObj.CustomMetadata,
	}
	if err := s.meta.PutObject(ctx, obj); err != nil {
		s.blob.Delete(ctx, dstBucket, dstKey)
		return "", err
	}
	return etag, nil
}
