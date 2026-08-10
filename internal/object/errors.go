package object

import (
	"errors"
	"fmt"
)

// Code is an S3-compatible error code.
type Code string

const (
	CodeNoSuchBucket       Code = "NoSuchBucket"
	CodeNoSuchKey          Code = "NoSuchKey"
	CodeBucketAlreadyExists Code = "BucketAlreadyExists"
	CodeBucketNotEmpty     Code = "BucketNotEmpty"
	CodeInvalidBucketName  Code = "InvalidBucketName"
	CodeInvalidArgument    Code = "InvalidArgument"
	CodeEntityTooLarge     Code = "EntityTooLarge"
	CodeEntityTooSmall     Code = "EntityTooSmall"
	CodeInternalError      Code = "InternalError"
	CodeNotImplemented     Code = "NotImplemented"
	CodeNoSuchUpload       Code = "NoSuchUpload"
	CodeInvalidPart        Code = "InvalidPart"
	CodeInvalidPartOrder   Code = "InvalidPartOrder"
	CodeMalformedXML       Code = "MalformedXML"
	CodeReplicationFailed  Code = "InternalError" // surfaced as InternalError to S3 clients
)

// Error is a domain error mapped to S3 XML responses.
type Error struct {
	Code       Code
	Message    string
	StatusCode int
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func newErr(code Code, message string, status int) *Error {
	return &Error{Code: code, Message: message, StatusCode: status}
}

// NotFoundKey returns a standard NoSuchKey error.
func NotFoundKey() *Error {
	return newErr(CodeNoSuchKey, "The specified key does not exist.", 404)
}

// NewReplicationError maps replication failures to a client-visible error.
func NewReplicationError(err error) *Error {
	return newErr(CodeInternalError, "Object replication did not complete on enough replicas.", 500)
}

// NewQuorumReadError maps read quorum failures to a client-visible error.
func NewQuorumReadError(err error) *Error {
	return newErr(CodeInternalError, "Unable to read object: read quorum not satisfied.", 503)
}

// AsError returns *Error if err is or wraps one.
func AsError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	var oe *Error
	if errors.As(err, &oe) {
		return oe, true
	}
	return nil, false
}
