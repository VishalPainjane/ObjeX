package validation

import (
	"regexp"
	"strings"
	"unicode"
)

const maxKeyLength = 1024

var bucketNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// BucketNameError returns a human-readable validation error for a bucket name, or nil if valid.
func BucketNameError(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Bucket name cannot be empty"
	}
	if len(name) < 3 {
		return "Bucket name must be at least 3 characters"
	}
	if len(name) > 63 {
		return "Bucket name must not exceed 63 characters"
	}
	if !isLowerOrDigit(name[0]) {
		return "Bucket name must start with a lowercase letter or number"
	}
	if !isLowerOrDigit(name[len(name)-1]) {
		return "Bucket name must end with a lowercase letter or number"
	}
	if strings.Contains(name, "..") {
		return "Bucket name cannot contain consecutive periods"
	}
	if !bucketNameRegex.MatchString(name) {
		return "Bucket name can only contain lowercase letters, numbers, and hyphens"
	}
	return ""
}

// KeyError returns a human-readable validation error for an object key, or nil if valid.
func KeyError(key string) string {
	if key == "" {
		return "Object key must not be empty"
	}
	if len(key) > maxKeyLength {
		return "Object key must not exceed 1024 characters"
	}
	if strings.HasPrefix(key, "/") {
		return "Object key must not start with '/'"
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return "Object key must not contain control characters"
		}
	}
	if SanitizeKey(key) == "" {
		return "Object key must not resolve to empty after normalization"
	}
	return ""
}

// SanitizeKey normalizes a key for physical path hashing (not stored logical key).
func SanitizeKey(key string) string {
	s := strings.ReplaceAll(key, "..", "")
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.Trim(s, "/")
	return s
}

func isLowerOrDigit(b byte) bool {
	return unicode.IsLower(rune(b)) || unicode.IsDigit(rune(b))
}
