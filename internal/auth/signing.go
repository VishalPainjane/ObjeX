package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// HmacSHA256 computes HMAC-SHA256.
func HmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// DeriveSigningKey performs the AWS4 signing key derivation chain.
func DeriveSigningKey(secretAccessKey, date, region, service string) []byte {
	kDate := HmacSHA256([]byte("AWS4"+secretAccessKey), []byte(date))
	kRegion := HmacSHA256(kDate, []byte(region))
	kService := HmacSHA256(kRegion, []byte(service))
	return HmacSHA256(kService, []byte("aws4_request"))
}

// SHA256Hex returns lowercase hex SHA256 of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HexEncode lowercases hex encoding of bytes.
func HexEncode(b []byte) string {
	return hex.EncodeToString(b)
}

// SignaturesEqual compares signatures in constant time.
func SignaturesEqual(expected, actual string) bool {
	return hmac.Equal([]byte(expected), []byte(actual))
}
