package hash

import (
	"crypto/md5"
	"encoding/hex"
	"testing"
)

func TestMultipartETag(t *testing.T) {
	// Two parts with known MD5 hex strings
	p1 := md5.Sum([]byte("part1"))
	p2 := md5.Sum([]byte("part2"))
	etags := []string{hex.EncodeToString(p1[:]), hex.EncodeToString(p2[:])}
	got, err := MultipartETag(etags)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSuffix(got, "-2") {
		t.Fatalf("etag = %q, want suffix -2", got)
	}
}

func hasSuffix(s, suf string) bool {
	return len(s) >= len(suf) && s[len(s)-len(suf):] == suf
}
