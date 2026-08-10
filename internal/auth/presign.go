package auth

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// PresignOptions configures presigned URL generation.
type PresignOptions struct {
	BaseURL        string
	Bucket         string
	Key            string
	AccessKeyID    string
	SecretAccessKey string
	ExpiresSeconds int
	Region         string
	Method         string
}

// GeneratePresignedURL builds a SigV4 presigned URL for GET or PUT.
func GeneratePresignedURL(opts PresignOptions) (string, error) {
	if opts.Region == "" {
		opts.Region = "us-east-1"
	}
	method := strings.ToUpper(opts.Method)
	if method == "" {
		method = "GET"
	}
	if opts.ExpiresSeconds <= 0 {
		opts.ExpiresSeconds = 3600
	}

	now := time.Now().UTC()
	timestamp := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", date, opts.Region)

	base := strings.TrimRight(opts.BaseURL, "/")
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	host := u.Host

	canonicalURI := "/" + UriEncodeStrict(opts.Bucket)
	if opts.Key != "" {
		for _, seg := range strings.Split(opts.Key, "/") {
			canonicalURI += "/" + UriEncodeStrict(seg)
		}
	}

	queryParams := map[string]string{
		"X-Amz-Algorithm":     "AWS4-HMAC-SHA256",
		"X-Amz-Credential":      fmt.Sprintf("%s/%s/%s/s3/aws4_request", opts.AccessKeyID, date, opts.Region),
		"X-Amz-Date":            timestamp,
		"X-Amz-Expires":         fmt.Sprintf("%d", opts.ExpiresSeconds),
		"X-Amz-SignedHeaders":   "host",
	}
	keys := make([]string, 0, len(queryParams))
	for k := range queryParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var canonicalParts []string
	for _, k := range keys {
		canonicalParts = append(canonicalParts, UriEncodeStrict(k)+"="+UriEncodeStrict(queryParams[k]))
	}
	canonicalQuery := strings.Join(canonicalParts, "&")

	canonicalHeaders := "host:" + host + "\n"
	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		timestamp,
		scope,
		SHA256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := DeriveSigningKey(opts.SecretAccessKey, date, opts.Region, "s3")
	signature := HexEncode(HmacSHA256(signingKey, []byte(stringToSign)))

	return fmt.Sprintf("%s%s?%s&X-Amz-Signature=%s", base, canonicalURI, canonicalQuery, signature), nil
}
