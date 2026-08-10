package object

import (
	"context"
	"errors"
	"io"
	"sort"

	"github.com/VishalPainjane/objex/internal/hash"
	"github.com/VishalPainjane/objex/internal/metadata"
	"github.com/VishalPainjane/objex/internal/validation"
)

// InitiateMultipartUpload starts a new multipart upload session.
func (s *Service) InitiateMultipartUpload(ctx context.Context, bucket, key, contentType string) (string, error) {
	if msg := validation.KeyError(key); msg != "" {
		return "", newErr(CodeInvalidArgument, msg, 400)
	}
	id, err := s.meta.CreateMultipartUpload(ctx, bucket, key, contentType)
	if errors.Is(err, metadata.ErrBucketNotFound) {
		return "", newErr(CodeNoSuchBucket, "The specified bucket does not exist.", 404)
	}
	return id, err
}

// UploadPart stores one part of a multipart upload.
func (s *Service) UploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int, body io.Reader) (string, error) {
	if partNumber < 1 || partNumber > 10000 {
		return "", newErr(CodeInvalidArgument, "Part number must be between 1 and 10000.", 400)
	}
	if msg := validation.KeyError(key); msg != "" {
		return "", newErr(CodeInvalidArgument, msg, 400)
	}

	upload, err := s.meta.GetMultipartUpload(ctx, uploadID, bucket, key)
	if errors.Is(err, metadata.ErrNoSuchUpload) {
		return "", newErr(CodeNoSuchUpload, "The specified upload does not exist.", 404)
	}
	if err != nil {
		return "", err
	}
	_ = upload

	if s.minFreeBytes > 0 && s.blob.AvailableFreeBytes() < s.minFreeBytes {
		return "", newErr(CodeEntityTooLarge, "Insufficient disk space.", 507)
	}

	limited := body
	if s.maxUploadBytes > 0 {
		limited = newLimitReader(body, s.maxUploadBytes)
	}

	partResult, err := s.blob.PutPart(ctx, uploadID, partNumber, limited)
	if err != nil {
		if errors.Is(err, errUploadTooLarge) {
			return "", newErr(CodeEntityTooLarge, "Part exceeds maximum upload size.", 400)
		}
		return "", err
	}

	part := metadata.MultipartPart{
		UploadID:    uploadID,
		PartNumber:  partNumber,
		Size:        partResult.Size,
		ETag:        partResult.ETag,
		StoragePath: partResult.Path,
	}
	if err := s.meta.UpsertMultipartPart(ctx, part); err != nil {
		return "", err
	}
	return partResult.ETag, nil
}

// CompleteMultipartUpload assembles parts into a final object.
func (s *Service) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, requested []metadata.CompletePart) (string, error) {
	if len(requested) == 0 {
		return "", newErr(CodeMalformedXML, "You must specify at least one part.", 400)
	}

	upload, err := s.meta.GetMultipartUpload(ctx, uploadID, bucket, key)
	if errors.Is(err, metadata.ErrNoSuchUpload) {
		return "", newErr(CodeNoSuchUpload, "The specified upload does not exist.", 404)
	}
	if err != nil {
		return "", err
	}

	storedParts, err := s.meta.ListMultipartParts(ctx, uploadID)
	if err != nil {
		return "", err
	}
	byNumber := make(map[int]metadata.MultipartPart, len(storedParts))
	for _, p := range storedParts {
		byNumber[p.PartNumber] = p
	}

	sort.Slice(requested, func(i, j int) bool {
		return requested[i].PartNumber < requested[j].PartNumber
	})
	for i := 1; i < len(requested); i++ {
		if requested[i].PartNumber <= requested[i-1].PartNumber {
			return "", newErr(CodeInvalidPartOrder, "Part numbers must be in ascending order.", 400)
		}
	}

	var orderedPaths []string
	var partETags []string
	var totalSize int64

	for i, req := range requested {
		stored, ok := byNumber[req.PartNumber]
		if !ok || stored.ETag != unquoteETag(req.ETag) {
			return "", newErr(CodeInvalidPart, "Part "+itoa(req.PartNumber)+" is invalid or ETag does not match.", 400)
		}
		if i < len(requested)-1 && stored.Size < minPartSize {
			return "", newErr(CodeEntityTooSmall, "Part "+itoa(req.PartNumber)+" is smaller than the minimum allowed size.", 400)
		}
		orderedPaths = append(orderedPaths, stored.StoragePath)
		partETags = append(partETags, stored.ETag)
		totalSize += stored.Size
	}

	if s.minFreeBytes > 0 && s.blob.AvailableFreeBytes() < s.minFreeBytes {
		return "", newErr(CodeEntityTooLarge, "Insufficient disk space.", 507)
	}

	putResult, err := s.blob.AssembleParts(ctx, bucket, key, orderedPaths)
	if err != nil {
		return "", err
	}

	finalETag, err := hash.MultipartETag(partETags)
	if err != nil {
		s.blob.Delete(ctx, bucket, key)
		return "", err
	}

	obj := metadata.Object{
		BucketName:  bucket,
		Key:         key,
		Size:        totalSize,
		ContentType: upload.ContentType,
		ETag:        finalETag,
		StoragePath: putResult.Path,
	}
	if err := s.meta.PutObject(ctx, obj); err != nil {
		s.blob.Delete(ctx, bucket, key)
		return "", err
	}

	s.blob.DeleteUploadParts(ctx, uploadID)
	s.meta.DeleteMultipartUpload(ctx, uploadID)

	return finalETag, nil
}

// AbortMultipartUpload cancels an in-progress upload.
func (s *Service) AbortMultipartUpload(ctx context.Context, bucket, key, uploadID string) error {
	_, err := s.meta.GetMultipartUpload(ctx, uploadID, bucket, key)
	if errors.Is(err, metadata.ErrNoSuchUpload) {
		return newErr(CodeNoSuchUpload, "The specified upload does not exist.", 404)
	}
	if err != nil {
		return err
	}
	if err := s.blob.DeleteUploadParts(ctx, uploadID); err != nil {
		return err
	}
	return s.meta.DeleteMultipartUpload(ctx, uploadID)
}

// ListParts returns parts for a multipart upload.
func (s *Service) ListParts(ctx context.Context, bucket, key, uploadID string) ([]metadata.MultipartPart, error) {
	_, err := s.meta.GetMultipartUpload(ctx, uploadID, bucket, key)
	if errors.Is(err, metadata.ErrNoSuchUpload) {
		return nil, newErr(CodeNoSuchUpload, "The specified upload does not exist.", 404)
	}
	if err != nil {
		return nil, err
	}
	return s.meta.ListMultipartParts(ctx, uploadID)
}

// ListMultipartUploads lists in-progress uploads in a bucket.
func (s *Service) ListMultipartUploads(ctx context.Context, bucket string) ([]metadata.MultipartUpload, error) {
	if ok, err := s.meta.BucketExists(ctx, bucket); err != nil {
		return nil, err
	} else if !ok {
		return nil, newErr(CodeNoSuchBucket, "The specified bucket does not exist.", 404)
	}
	return s.meta.ListMultipartUploads(ctx, bucket)
}

func unquoteETag(etag string) string {
	etag = trimSpace(etag)
	if len(etag) >= 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		return etag[1 : len(etag)-1]
	}
	return etag
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
