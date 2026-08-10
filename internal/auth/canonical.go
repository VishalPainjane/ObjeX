package auth

import (
	"crypto/hmac"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// ParsedSig holds parsed SigV4 credential information.
type ParsedSig struct {
	AccessKeyID   string
	Date          string // YYYYMMDD
	Region        string
	Service       string
	SignedHeaders []string
	Signature     string
	IsPresigned   bool
}

// RequestContext captures HTTP data needed for canonical request construction.
type RequestContext struct {
	Method              string
	Path                string
	RawQuery            string
	Host                string
	Headers             http.Header
	Query               url.Values
	IsPresigned         bool
	PayloadHashDeclared string
}

// RequestContextFromHTTP builds RequestContext from an HTTP request.
func RequestContextFromHTTP(r *http.Request) RequestContext {
	host := r.Host
	if host == "" {
		host = r.Header.Get("Host")
	}
	declared := r.Header.Get("x-amz-content-sha256")
	return RequestContext{
		Method:              r.Method,
		Path:                r.URL.Path,
		RawQuery:            r.URL.RawQuery,
		Host:                host,
		Headers:             r.Header,
		Query:               r.URL.Query(),
		IsPresigned:         r.URL.Query().Has("X-Amz-Algorithm"),
		PayloadHashDeclared: declared,
	}
}

// BuildCanonicalRequest constructs the SigV4 canonical request string.
func BuildCanonicalRequest(ctx RequestContext, parsed ParsedSig) string {
	method := strings.ToUpper(ctx.Method)
	canonicalURI := CanonicalURI(ctx.Path)
	canonicalQuery := CanonicalQueryString(ctx, parsed)
	canonicalHeaders := CanonicalHeaders(ctx, parsed.SignedHeaders)
	signedHeaders := strings.Join(parsed.SignedHeaders, ";")
	payloadHash := PayloadHash(ctx)

	return strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
}

// CanonicalURI decodes then re-encodes each path segment.
func CanonicalURI(path string) string {
	decoded, err := url.PathUnescape(path)
	if err != nil {
		decoded = path
	}
	if decoded == "" {
		decoded = "/"
	}
	segments := strings.Split(decoded, "/")
	for i, seg := range segments {
		segments[i] = UriEncodeStrict(seg)
	}
	return strings.Join(segments, "/")
}

// CanonicalQueryString builds the sorted, encoded query string.
func CanonicalQueryString(ctx RequestContext, parsed ParsedSig) string {
	var pairs []queryPair

	if ctx.IsPresigned {
		for key, vals := range ctx.Query {
			if key == "X-Amz-Signature" {
				continue
			}
			for _, v := range vals {
				pairs = append(pairs, queryPair{
					key:   UriEncodeStrict(key),
					value: UriEncodeStrict(v),
				})
			}
		}
	} else if ctx.RawQuery != "" {
		raw := ctx.RawQuery
		for _, part := range strings.Split(raw, "&") {
			if part == "" {
				continue
			}
			key, val, _ := strings.Cut(part, "=")
			key = UriEncodeStrict(decodeQueryComponent(key))
			val = UriEncodeStrict(decodeQueryComponent(val))
			pairs = append(pairs, queryPair{key: key, value: val})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key != pairs[j].key {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].value < pairs[j].value
	})

	if len(pairs) == 0 {
		return ""
	}
	var parts []string
	for _, p := range pairs {
		parts = append(parts, p.key+"="+p.value)
	}
	return strings.Join(parts, "&")
}

type queryPair struct {
	key, value string
}

func decodeQueryComponent(s string) string {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// CanonicalHeaders builds canonical headers block for signed header names.
func CanonicalHeaders(ctx RequestContext, signedHeaders []string) string {
	var sb strings.Builder
	for _, name := range signedHeaders {
		name = strings.ToLower(strings.TrimSpace(name))
		var value string
		if name == "host" {
			value = ctx.Host
		} else {
			vals := ctx.Headers.Values(name)
			if len(vals) == 0 {
				// Try canonical MIME header key
				vals = ctx.Headers.Values(http.CanonicalHeaderKey(name))
			}
			cleaned := make([]string, len(vals))
			for i, v := range vals {
				cleaned[i] = CollapseSpaces(strings.TrimSpace(v))
			}
			value = strings.Join(cleaned, ",")
		}
		sb.WriteString(name)
		sb.WriteByte(':')
		sb.WriteString(value)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// PayloadHash returns the payload hash for the canonical request.
func PayloadHash(ctx RequestContext) string {
	declared := ctx.PayloadHashDeclared
	if declared == "" {
		return "UNSIGNED-PAYLOAD"
	}
	return declared
}

// BuildStringToSign constructs the SigV4 string to sign.
func BuildStringToSign(ctx RequestContext, parsed ParsedSig, canonicalRequest string) string {
	timestamp := RequestTimestamp(ctx, parsed.Date)
	scope := fmt.Sprintf("%s/%s/%s/aws4_request", parsed.Date, parsed.Region, parsed.Service)
	hash := SHA256Hex([]byte(canonicalRequest))
	return strings.Join([]string{
		"AWS4-HMAC-SHA256",
		timestamp,
		scope,
		hash,
	}, "\n")
}

// RequestTimestamp extracts x-amz-date from headers or query.
func RequestTimestamp(ctx RequestContext, date string) string {
	ts := strings.TrimSpace(ctx.Headers.Get("x-amz-date"))
	if ts == "" && ctx.Query != nil {
		ts = strings.TrimSpace(ctx.Query.Get("X-Amz-Date"))
	}
	if ts == "" && len(date) == 8 {
		return date + "T000000Z"
	}
	return ts
}

// VerifySignature checks the client signature against the expected value.
func VerifySignature(secretAccessKey string, parsed ParsedSig, ctx RequestContext) (bool, string, string) {
	canonical := BuildCanonicalRequest(ctx, parsed)
	stringToSign := BuildStringToSign(ctx, parsed, canonical)
	signingKey := DeriveSigningKey(secretAccessKey, parsed.Date, parsed.Region, parsed.Service)
	expected := HexEncode(HmacSHA256(signingKey, []byte(stringToSign)))
	valid := signaturesMatch(expected, parsed.Signature)
	return valid, expected, canonical
}

func signaturesMatch(expected, actual string) bool {
	exp, err1 := hexDecodeSignature(expected)
	act, err2 := hexDecodeSignature(actual)
	if err1 != nil || err2 != nil {
		return false
	}
	return hmac.Equal(exp, act)
}

func hexDecodeSignature(sig string) ([]byte, error) {
	sig = strings.ToLower(strings.TrimSpace(sig))
	return hex.DecodeString(sig)
}
