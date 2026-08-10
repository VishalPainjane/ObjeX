package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/VishalPainjane/objex/internal/validation"
)

// ObjectAddressHash returns the SHA256 hex digest used for physical blob paths.
// Input is "{bucketName}/{sanitizedKey}" per V1 layout.
func ObjectAddressHash(bucket, key string) string {
	input := bucket + "/" + validation.SanitizeKey(key)
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

// BlobRelativePath returns the relative path under the storage root for an object.
func BlobRelativePath(bucket, key string) string {
	hash := ObjectAddressHash(bucket, key)
	l1 := hash[0:2]
	l2 := hash[2:4]
	return filepath.Join(bucket, l1, l2, hash+".blob")
}

// ResolveBlobPath joins basePath with the relative blob path and validates containment.
func ResolveBlobPath(basePath, bucket, key string) (string, error) {
	rel := BlobRelativePath(bucket, key)
	full := filepath.Join(basePath, rel)
	return AssertWithinBase(full, basePath)
}

// AssertWithinBase ensures resolved path stays under basePath.
func AssertWithinBase(fullPath, basePath string) (string, error) {
	resolved, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	baseResolved, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	basePrefix := baseResolved + string(filepath.Separator)
	if resolved != baseResolved && !strings.HasPrefix(resolved, basePrefix) {
		return "", fmt.Errorf("%w: %s", ErrPathEscape, resolved)
	}
	return resolved, nil
}
