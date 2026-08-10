package auth

import (
	"errors"
	"net/http"
	"strings"
)

// AuthError is an authentication failure with an S3 error code.
type AuthError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *AuthError) Error() string {
	return e.Code + ": " + e.Message
}

// S3 error code constants matching V1.
const (
	ErrAccessDenied        = "AccessDenied"
	ErrInvalidAccessKeyId  = "InvalidAccessKeyId"
	ErrInvalidArgument     = "InvalidArgument"
	ErrSignatureDoesNotMatch = "SignatureDoesNotMatch"
	ErrRequestExpired      = "RequestExpired"
)

// ParseSigV4 extracts SigV4 credentials from the request.
// Returns nil if no credentials are present (not an error).
func ParseSigV4(r *http.Request) (*ParsedSig, error) {
	if r.URL.Query().Has("X-Amz-Algorithm") {
		return parseQuerySig(r)
	}

	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil, nil
	}
	if !strings.HasPrefix(strings.ToUpper(auth), "AWS4-HMAC-SHA256 ") {
		return nil, &AuthError{
			Code:       ErrInvalidArgument,
			Message:    "Only AWS4-HMAC-SHA256 is supported.",
			StatusCode: http.StatusBadRequest,
		}
	}
	return parseAuthHeader(auth[len("AWS4-HMAC-SHA256 "):])
}

func parseAuthHeader(value string) (*ParsedSig, error) {
	parts := strings.Split(value, ",")
	var credential, signedHeaders, signature string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if after, ok := strings.CutPrefix(part, "Credential="); ok {
			credential = after
		} else if after, ok := strings.CutPrefix(part, "SignedHeaders="); ok {
			signedHeaders = after
		} else if after, ok := strings.CutPrefix(part, "Signature="); ok {
			signature = after
		}
	}
	if credential == "" || signedHeaders == "" || signature == "" {
		return nil, &AuthError{
			Code:       ErrInvalidArgument,
			Message:    "Malformed Authorization header.",
			StatusCode: http.StatusBadRequest,
		}
	}
	return parseCredential(credential, strings.Split(signedHeaders, ";"), signature, false)
}

func parseQuerySig(r *http.Request) (*ParsedSig, error) {
	credential := r.URL.Query().Get("X-Amz-Credential")
	signedHeaders := r.URL.Query().Get("X-Amz-SignedHeaders")
	signature := r.URL.Query().Get("X-Amz-Signature")
	if credential == "" || signedHeaders == "" || signature == "" {
		return nil, &AuthError{
			Code:       ErrInvalidArgument,
			Message:    "Missing presigned URL parameters.",
			StatusCode: http.StatusBadRequest,
		}
	}
	return parseCredential(credential, strings.Split(signedHeaders, ";"), signature, true)
}

func parseCredential(credential string, signedHeaders []string, signature string, presigned bool) (*ParsedSig, error) {
	segments := strings.Split(credential, "/")
	if len(segments) < 5 {
		return nil, &AuthError{
			Code:       ErrInvalidAccessKeyId,
			Message:    "Invalid credential scope.",
			StatusCode: http.StatusForbidden,
		}
	}
	return &ParsedSig{
		AccessKeyID:   segments[0],
		Date:          segments[1],
		Region:        segments[2],
		Service:       segments[3],
		SignedHeaders: signedHeaders,
		Signature:     signature,
		IsPresigned:   presigned,
	}, nil
}

// IsAuthError reports whether err is an AuthError.
func IsAuthError(err error) (*AuthError, bool) {
	if err == nil {
		return nil, false
	}
	var ae *AuthError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
