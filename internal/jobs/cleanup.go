package jobs

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/VishalPainjane/objex/internal/metadata"
	"github.com/VishalPainjane/objex/internal/storage/filesystem"
)

const abandonedMultipartAge = 7 * 24 * time.Hour

// Cleanup runs background maintenance tasks.
type Cleanup struct {
	meta   metadata.Store
	blob   *filesystem.Storage
	logger *slog.Logger
}

// NewCleanup creates a cleanup job runner.
func NewCleanup(meta metadata.Store, blob *filesystem.Storage, logger *slog.Logger) *Cleanup {
	if logger == nil {
		logger = slog.Default()
	}
	return &Cleanup{meta: meta, blob: blob, logger: logger}
}

// RunAbandonedMultipart deletes multipart uploads older than 7 days and their part files.
func (c *Cleanup) RunAbandonedMultipart(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-abandonedMultipartAge)
	deleted, err := c.meta.DeleteAbandonedMultipartUploads(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	// Part rows cascade; remove any leftover dirs on disk.
	multipartRoot := filepath.Join(c.blob.BasePath(), "_multipart")
	entries, err := os.ReadDir(multipartRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return deleted, nil
		}
		return deleted, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		dir := filepath.Join(multipartRoot, e.Name())
		if err := os.RemoveAll(dir); err != nil {
			c.logger.Warn("failed to delete abandoned multipart dir", "path", dir, "error", err)
		}
	}
	return deleted, nil
}

// RunOrphanBlobs deletes blob files not referenced in metadata.
func (c *Cleanup) RunOrphanBlobs(ctx context.Context) (int, error) {
	known, err := c.meta.AllObjectStoragePaths(ctx)
	if err != nil {
		return 0, err
	}
	knownSet := make(map[string]struct{}, len(known))
	for _, p := range known {
		knownSet[p] = struct{}{}
	}
	if hintPaths, err := c.meta.ListHintProtectedStoragePaths(ctx); err == nil {
		for _, p := range hintPaths {
			knownSet[p] = struct{}{}
		}
	}

	base := c.blob.BasePath()
	deleted := 0
	err = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == "_multipart" || info.Name() == "_hints" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".blob") {
			return nil
		}
		if _, ok := knownSet[path]; ok {
			return nil
		}
		if err := os.Remove(path); err != nil {
			c.logger.Warn("failed to delete orphan blob", "path", path, "error", err)
			return nil
		}
		deleted++
		return nil
	})
	return deleted, err
}

// StartPeriodic runs cleanup on a schedule until ctx is cancelled.
func (c *Cleanup) StartPeriodic(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.runAll(ctx)
			}
		}
	}()
}

func (c *Cleanup) runAll(ctx context.Context) {
	if n, err := c.RunOrphanBlobs(ctx); err != nil {
		c.logger.Error("orphan blob cleanup failed", "error", err)
	} else if n > 0 {
		c.logger.Info("orphan blob cleanup", "deleted", n)
	}
	if n, err := c.RunAbandonedMultipart(ctx); err != nil {
		c.logger.Error("abandoned multipart cleanup failed", "error", err)
	} else if n > 0 {
		c.logger.Info("abandoned multipart cleanup", "deleted", n)
	}
	if _, err := c.RunIntegrityVerify(ctx); err != nil {
		c.logger.Error("integrity verification failed", "error", err)
	}
}
