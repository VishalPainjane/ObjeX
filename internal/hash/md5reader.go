package hash

import (
	"crypto/md5"
	"encoding/hex"
	"hash"
	"io"
)

// MD5Reader computes an MD5 digest while reading from the underlying reader.
type MD5Reader struct {
	r   io.Reader
	h   hash.Hash
	n   int64
	err error
}

// NewMD5Reader wraps r for streaming MD5 computation.
func NewMD5Reader(r io.Reader) *MD5Reader {
	return &MD5Reader{r: r, h: md5.New()}
}

// Read implements io.Reader.
func (m *MD5Reader) Read(p []byte) (int, error) {
	n, err := m.r.Read(p)
	if n > 0 {
		if _, werr := m.h.Write(p[:n]); werr != nil {
			m.err = werr
			return n, werr
		}
		m.n += int64(n)
	}
	if err != nil {
		m.err = err
	}
	return n, err
}

// Size returns bytes read so far.
func (m *MD5Reader) Size() int64 {
	return m.n
}

// ETag returns lowercase hex MD5 of all bytes read.
func (m *MD5Reader) ETag() string {
	return hex.EncodeToString(m.h.Sum(nil))
}

// Err returns the first read error other than EOF.
func (m *MD5Reader) Err() error {
	if m.err != nil && m.err != io.EOF {
		return m.err
	}
	return nil
}
