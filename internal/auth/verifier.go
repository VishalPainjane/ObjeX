package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"
)

const clockSkewMinutes = 15

// Credential holds S3 API credentials.
type Credential struct {
	AccessKeyID     string
	SecretAccessKey string
}

// CredentialStore resolves credentials by access key ID.
type CredentialStore interface {
	GetCredential(ctx context.Context, accessKeyID string) (*Credential, error)
}

// Verifier validates SigV4 requests.
type Verifier struct {
	Region string
	Store  CredentialStore
}

// VerifyRequest authenticates an HTTP request and returns the matched credential.
func (v *Verifier) VerifyRequest(ctx context.Context, r *http.Request) (*Credential, error) {
	parsed, err := ParseSigV4(r)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, &AuthError{
			Code:       ErrAccessDenied,
			Message:    "No credentials provided.",
			StatusCode: http.StatusForbidden,
		}
	}

	cred, err := v.Store.GetCredential(ctx, parsed.AccessKeyID)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, &AuthError{
			Code:       ErrInvalidAccessKeyId,
			Message:    "The AWS access key Id you provided does not exist.",
			StatusCode: http.StatusForbidden,
		}
	}

	reqCtx := RequestContextFromHTTP(r)
	if !isTimestampFresh(r, parsed.Date) {
		return nil, &AuthError{
			Code:       ErrRequestExpired,
			Message:    "Request has expired. Check your system clock.",
			StatusCode: http.StatusForbidden,
		}
	}

	valid, _, _ := VerifySignature(cred.SecretAccessKey, *parsed, reqCtx)
	if !valid {
		return nil, &AuthError{
			Code:       ErrSignatureDoesNotMatch,
			Message:    "The request signature we calculated does not match the signature you provided.",
			StatusCode: http.StatusForbidden,
		}
	}

	if err := verifyPayloadHash(r, reqCtx); err != nil {
		return nil, err
	}

	return cred, nil
}

func isTimestampFresh(r *http.Request, credentialDate string) bool {
	raw := strings.TrimSpace(r.Header.Get("x-amz-date"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("X-Amz-Date"))
	}
	signingTime, err := time.Parse("20060102T150405Z", raw)
	if err != nil {
		return false
	}
	now := time.Now().UTC()

	if r.URL.Query().Has("X-Amz-Expires") {
		expiresSec, err := parseInt(r.URL.Query().Get("X-Amz-Expires"))
		if err != nil || expiresSec < 0 {
			return false
		}
		return !now.Before(signingTime) && !now.After(signingTime.Add(time.Duration(expiresSec) * time.Second))
	}

	diff := now.Sub(signingTime)
	if diff < 0 {
		diff = -diff
	}
	return diff <= time.Duration(clockSkewMinutes)*time.Minute
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, io.ErrUnexpectedEOF
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func verifyPayloadHash(r *http.Request, ctx RequestContext) error {
	declared := strings.TrimSpace(ctx.PayloadHashDeclared)
	if declared == "" ||
		strings.EqualFold(declared, "UNSIGNED-PAYLOAD") ||
		strings.HasPrefix(strings.ToUpper(declared), "STREAMING-") {
		return nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	actual := sha256.Sum256(body)
	actualHex := hex.EncodeToString(actual[:])
	if !strings.EqualFold(declared, actualHex) {
		return &AuthError{
			Code:       ErrSignatureDoesNotMatch,
			Message:    "The payload hash does not match x-amz-content-sha256.",
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}
