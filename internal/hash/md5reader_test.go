package hash

import (
	"bytes"
	"crypto/md5"
	"testing"
)

func TestMD5ReaderETag(t *testing.T) {
	data := []byte("Hello, ObjeX round-trip test!")
	r := NewMD5Reader(bytes.NewReader(data))
	buf := make([]byte, 32)
	for {
		n, err := r.Read(buf)
		if n == 0 && err != nil {
			break
		}
	}
	want := md5.Sum(data)
	got := r.ETag()
	if got != hexEncode(want[:]) {
		t.Errorf("ETag = %q, want %q", got, hexEncode(want[:]))
	}
	if r.Size() != int64(len(data)) {
		t.Errorf("Size = %d", r.Size())
	}
}

func hexEncode(b []byte) string {
	const hextable = "0123456789abcdef"
	dst := make([]byte, len(b)*2)
	for i, v := range b {
		dst[i*2] = hextable[v>>4]
		dst[i*2+1] = hextable[v&0x0f]
	}
	return string(dst)
}
