package object

import (
	"errors"
	"io"
)

// errUploadTooLarge is returned when the upload exceeds the configured limit.
var errUploadTooLarge = errors.New("upload exceeds maximum size")

// limitReader allows up to limit bytes; an additional byte triggers errUploadTooLarge.
type limitReader struct {
	r     io.Reader
	limit int64
	n     int64
}

func newLimitReader(r io.Reader, limit int64) *limitReader {
	return &limitReader{r: r, limit: limit}
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.n > l.limit {
		return 0, errUploadTooLarge
	}
	remaining := l.limit - l.n
	if remaining == 0 {
		var probe [1]byte
		_, err := l.r.Read(probe[:])
		if err == io.EOF {
			return 0, io.EOF
		}
		if err != nil {
			return 0, err
		}
		return 0, errUploadTooLarge
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := l.r.Read(p)
	l.n += int64(n)
	return n, err
}
