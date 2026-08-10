package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/VishalPainjane/objex/internal/storage"
)

const (
	staleTmpThreshold       = time.Hour
	staleMultipartThreshold = 48 * time.Hour
)

// Storage implements storage.Storage on the local filesystem.
type Storage struct {
	basePath string
	logger   *slog.Logger
}

// New creates a filesystem storage backend at basePath.
func New(basePath string, logger *slog.Logger) (*Storage, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve base path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create base path: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &Storage{basePath: abs, logger: logger}
	s.cleanupStaleTmpFiles()
	return s, nil
}

// BasePath returns the absolute storage root.
func (s *Storage) BasePath() string {
	return s.basePath
}

func (s *Storage) Put(ctx context.Context, bucket, key string, body io.Reader) (storage.PutResult, error) {
	filePath, err := storage.ResolveBlobPath(s.basePath, bucket, key)
	if err != nil {
		return storage.PutResult{}, err
	}

	tmpPath, err := uniqueTmpPath(filePath)
	if err != nil {
		return storage.PutResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return storage.PutResult{}, fmt.Errorf("create object dir: %w", err)
	}

	size, err := writeToTmp(ctx, tmpPath, body)
	if err != nil {
		os.Remove(tmpPath)
		return storage.PutResult{}, err
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return storage.PutResult{}, fmt.Errorf("atomic rename: %w", err)
	}

	return storage.PutResult{Path: filePath, Size: size}, nil
}

func (s *Storage) Get(ctx context.Context, bucket, key string) (io.ReadCloser, storage.Info, error) {
	filePath, err := storage.ResolveBlobPath(s.basePath, bucket, key)
	if err != nil {
		return nil, storage.Info{}, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, storage.Info{}, storage.ErrNotFound
		}
		return nil, storage.Info{}, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, storage.Info{}, err
	}

	return f, storage.Info{Path: filePath, Size: info.Size()}, nil
}

func (s *Storage) Delete(ctx context.Context, bucket, key string) error {
	filePath, err := storage.ResolveBlobPath(s.basePath, bucket, key)
	if err != nil {
		return err
	}
	if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Storage) Stat(ctx context.Context, bucket, key string) (storage.Info, error) {
	filePath, err := storage.ResolveBlobPath(s.basePath, bucket, key)
	if err != nil {
		return storage.Info{}, err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storage.Info{}, storage.ErrNotFound
		}
		return storage.Info{}, err
	}
	return storage.Info{Path: filePath, Size: info.Size()}, nil
}

func (s *Storage) Exists(ctx context.Context, bucket, key string) (bool, error) {
	filePath, err := storage.ResolveBlobPath(s.basePath, bucket, key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s *Storage) AvailableFreeBytes() int64 {
	return availableFreeBytes(s.basePath)
}

func writeToTmp(ctx context.Context, tmpPath string, body io.Reader) (int64, error) {
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("create tmp file: %w", err)
	}
	defer f.Close()

	written, err := copyWithContext(ctx, f, body)
	if err != nil {
		return 0, err
	}
	if err := f.Sync(); err != nil {
		return 0, fmt.Errorf("sync tmp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	return written, nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			wn, writeErr := dst.Write(buf[:n])
			written += int64(wn)
			if writeErr != nil {
				return written, writeErr
			}
			if wn != n {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func uniqueTmpPath(finalPath string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate tmp suffix: %w", err)
	}
	return finalPath + "." + hex.EncodeToString(b[:]) + ".tmp", nil
}

func (s *Storage) cleanupStaleTmpFiles() {
	cutoff := time.Now().Add(-staleTmpThreshold)
	deleted := 0

	_ = filepath.Walk(s.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".tmp") {
			return nil
		}
		if info.ModTime().After(cutoff) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			s.logger.Warn("failed to delete stale tmp", "path", path, "error", err)
			return nil
		}
		deleted++
		return nil
	})

	if deleted > 0 {
		s.logger.Info("deleted stale tmp files on startup", "count", deleted)
	}

	multipartRoot := filepath.Join(s.basePath, "_multipart")
	entries, err := os.ReadDir(multipartRoot)
	if err != nil {
		return
	}
	cutoff2 := time.Now().Add(-staleMultipartThreshold)
	deletedDirs := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff2) {
			continue
		}
		dir := filepath.Join(multipartRoot, e.Name())
		if err := os.RemoveAll(dir); err != nil {
			s.logger.Warn("failed to delete stale multipart dir", "path", dir, "error", err)
			continue
		}
		deletedDirs++
	}
	if deletedDirs > 0 {
		s.logger.Info("deleted stale multipart dirs on startup", "count", deletedDirs)
	}
}
