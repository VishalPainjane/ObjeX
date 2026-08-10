package hash

import (
	"crypto/md5"
	"encoding/hex"
)

// MultipartETag computes the S3 multipart ETag: MD5(concat part MD5 bytes) + "-" + count.
func MultipartETag(partETags []string) (string, error) {
	h := md5.New()
	for _, etag := range partETags {
		b, err := hex.DecodeString(etag)
		if err != nil {
			return "", err
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)) + "-" + itoa(len(partETags)), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
