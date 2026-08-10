package auth

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const defaultSignRegion = "us-east-1"
const defaultSignService = "s3"

// SignRequest adds AWS SigV4 headers to an HTTP request for testing and clients.
func SignRequest(req *http.Request, accessKeyID, secretAccessKey string, body []byte) error {
	return SignRequestWithTime(req, accessKeyID, secretAccessKey, time.Now().UTC(), body)
}

// SignRequestWithTime signs a request with a specific timestamp.
func SignRequestWithTime(req *http.Request, accessKeyID, secretAccessKey string, ts time.Time, body []byte) error {
	payloadHash := "UNSIGNED-PAYLOAD"
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		payloadHash = hexEncode(sum[:])
	}

	date := ts.Format("20060102")
	amzDate := ts.Format("20060102T150405Z")

	host := req.Host
	if host == "" {
		host = req.Header.Get("Host")
	}
	if host == "" {
		if req.URL.IsAbs() {
			host = req.URL.Host
		} else {
			host = "localhost"
		}
	}
	req.Host = host
	req.Header.Set("Host", host)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	signedHeadersList := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	signedHeaders := strings.Join(signedHeadersList, ";")

	rawPath := req.URL.EscapedPath()
	if rawPath == "" {
		rawPath = "/"
	}
	canonicalURI := CanonicalURI(rawPath)

	rawQuery := req.URL.RawQuery
	canonicalQuery := buildCanonicalQueryFromRaw(rawQuery)

	var canonicalHeaders strings.Builder
	for _, name := range signedHeadersList {
		var val string
		switch name {
		case "host":
			val = host
		case "x-amz-content-sha256":
			val = payloadHash
		case "x-amz-date":
			val = amzDate
		}
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(val)
		canonicalHeaders.WriteByte('\n')
	}

	canonicalRequest := strings.Join([]string{
		strings.ToUpper(req.Method),
		canonicalURI,
		canonicalQuery,
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := fmt.Sprintf("%s/%s/%s/aws4_request", date, defaultSignRegion, defaultSignService)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		SHA256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := DeriveSigningKey(secretAccessKey, date, defaultSignRegion, defaultSignService)
	signature := HexEncode(HmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf(
		"Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKeyID, scope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 "+authHeader)
	return nil
}

func buildCanonicalQueryFromRaw(query string) string {
	query = strings.TrimPrefix(query, "?")
	if query == "" {
		return ""
	}
	var pairs []queryPair
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		key, val, _ := strings.Cut(part, "=")
		key = UriEncodeStrict(decodeQueryComponent(key))
		val = UriEncodeStrict(decodeQueryComponent(val))
		pairs = append(pairs, queryPair{key: key, value: val})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key != pairs[j].key {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].value < pairs[j].value
	})
	var parts []string
	for _, p := range pairs {
		parts = append(parts, p.key+"="+p.value)
	}
	return strings.Join(parts, "&")
}

func hexEncode(b []byte) string {
	return HexEncode(b)
}

// ParsePresignedURL parses a presigned URL into method, path, and query for HTTP requests.
func ParsePresignedURL(rawURL string) (*http.Request, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return http.NewRequest(http.MethodGet, u.String(), nil)
}
