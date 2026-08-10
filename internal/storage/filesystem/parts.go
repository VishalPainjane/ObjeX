package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/VishalPainjane/objex/internal/hash"
	"github.com/VishalPainjane/objex/internal/storage"
)

func (s *Storage) PutPart(ctx context.Context, uploadID string, partNumber int, body io.Reader) (storage.PartPutResult, error) {
	dir := filepath.Join(s.basePath, "_multipart", uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return storage.PartPutResult{}, fmt.Errorf("create multipart dir: %w", err)
	}
	partPath := filepath.Join(dir, fmt.Sprintf("%d.part", partNumber))

	tmpPath, err := uniqueTmpPath(partPath)
	if err != nil {
		return storage.PartPutResult{}, err
	}

	hasher := hash.NewMD5Reader(body)
	size, err := writeToTmp(ctx, tmpPath, hasher)
	if err != nil {
		os.Remove(tmpPath)
		return storage.PartPutResult{}, err
	}
	if herr := hasher.Err(); herr != nil && herr != io.EOF {
		os.Remove(tmpPath)
		return storage.PartPutResult{}, herr
	}

	if err := os.Rename(tmpPath, partPath); err != nil {
		os.Remove(tmpPath)
		return storage.PartPutResult{}, fmt.Errorf("atomic rename part: %w", err)
	}

	return storage.PartPutResult{
		Path: partPath,
		Size: size,
		ETag: hasher.ETag(),
	}, nil
}

func (s *Storage) AssembleParts(ctx context.Context, bucket, key string, orderedPartPaths []string) (storage.PutResult, error) {
	finalPath, err := storage.ResolveBlobPath(s.basePath, bucket, key)
	if err != nil {
		return storage.PutResult{}, err
	}

	tmpPath, err := uniqueTmpPath(finalPath)
	if err != nil {
		return storage.PutResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return storage.PutResult{}, err
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return storage.PutResult{}, err
	}

	var totalSize int64
	for _, partPath := range orderedPartPaths {
		if err := ctx.Err(); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return storage.PutResult{}, err
		}
		pf, err := os.Open(partPath)
		if err != nil {
			f.Close()
			os.Remove(tmpPath)
			return storage.PutResult{}, fmt.Errorf("open part %s: %w", partPath, err)
		}
		n, err := copyWithContext(ctx, f, pf)
		pf.Close()
		if err != nil {
			f.Close()
			os.Remove(tmpPath)
			return storage.PutResult{}, err
		}
		totalSize += n
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return storage.PutResult{}, err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return storage.PutResult{}, err
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return storage.PutResult{}, fmt.Errorf("atomic rename: %w", err)
	}

	return storage.PutResult{Path: finalPath, Size: totalSize}, nil
}

func (s *Storage) DeleteUploadParts(ctx context.Context, uploadID string) error {
	dir := filepath.Join(s.basePath, "_multipart", uploadID)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
