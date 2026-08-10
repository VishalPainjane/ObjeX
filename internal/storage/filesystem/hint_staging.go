package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/VishalPainjane/objex/internal/storage"
)

// OpenPath opens an absolute blob file path under the storage base.
func (s *Storage) OpenPath(path string) (io.ReadCloser, int64, error) {
	if !s.isUnderBase(path) {
		return nil, 0, fmt.Errorf("path outside storage base")
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, storage.ErrNotFound
		}
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// StageHintCopy duplicates object bytes into a durable hint payload path.
func (s *Storage) StageHintCopy(srcPath, targetNode, bucket, key string, version int64) (string, int64, error) {
	if !s.isUnderBase(srcPath) {
		return "", 0, fmt.Errorf("source path outside storage base")
	}
	dest := hintPayloadPath(s.basePath, targetNode, bucket, key, version)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", 0, err
	}
	size, err := copyFile(srcPath, dest)
	if err != nil {
		os.Remove(dest)
		return "", 0, err
	}
	return dest, size, nil
}

// RemovePath deletes a file under the storage base (used after hint delivery).
func (s *Storage) RemovePath(path string) error {
	if path == "" || !s.isUnderBase(path) {
		return nil
	}
	err := os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Storage) isUnderBase(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	base := filepath.Clean(s.basePath)
	return abs == base || strings.HasPrefix(abs, base+string(os.PathSeparator))
}

func hintPayloadPath(base, targetNode, bucket, key string, version int64) string {
	safeKey := strings.ReplaceAll(key, "/", "_")
	return filepath.Join(base, "_hints", targetNode, bucket, fmt.Sprintf("%d", version), safeKey+".blob")
}

func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	n, err := io.Copy(out, in)
	if err != nil {
		return 0, err
	}
	return n, out.Close()
}
