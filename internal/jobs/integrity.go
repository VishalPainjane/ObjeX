package jobs

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"time"
)

// IntegrityResult summarizes a blob integrity verification pass.
type IntegrityResult struct {
	Checked  int
	Corrupted int
	Missing  int
	Duration time.Duration
}

// RunIntegrityVerify re-hashes stored blobs and compares against metadata ETags.
func (c *Cleanup) RunIntegrityVerify(ctx context.Context) (IntegrityResult, error) {
	start := time.Now()
	result := IntegrityResult{}

	objects, err := c.meta.ListAllStoredObjects(ctx)
	if err != nil {
		return result, err
	}

	for _, obj := range objects {
		if obj.StoragePath == "" || obj.ETag == "" {
			continue
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		f, err := os.Open(obj.StoragePath)
		if err != nil {
			if os.IsNotExist(err) {
				result.Missing++
				c.logger.Error("blob missing for integrity check",
					"bucket", obj.BucketName, "key", obj.Key, "path", obj.StoragePath)
				continue
			}
			return result, err
		}

		hash := md5.New()
		if _, err := io.Copy(hash, f); err != nil {
			f.Close()
			return result, err
		}
		f.Close()

		actual := hex.EncodeToString(hash.Sum(nil))
		result.Checked++
		if !strings.EqualFold(actual, obj.ETag) {
			result.Corrupted++
			c.logger.Error("blob integrity mismatch",
				"bucket", obj.BucketName, "key", obj.Key,
				"stored_etag", obj.ETag, "actual_etag", actual)
		}
	}

	result.Duration = time.Since(start)
	if result.Corrupted > 0 || result.Missing > 0 {
		c.logger.Warn("integrity verification finished",
			"checked", result.Checked, "corrupted", result.Corrupted,
			"missing", result.Missing, "duration", result.Duration)
	} else if result.Checked > 0 {
		c.logger.Info("integrity verification finished",
			"checked", result.Checked, "duration", result.Duration)
	}
	return result, nil
}
