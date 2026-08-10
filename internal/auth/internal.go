package auth

import "net/http"

const InternalTokenHeader = "X-ObjeX-Internal-Token"

// Internal operation headers — require valid internal token (authentication separate from semantics).
const (
	InternalOperationHeader = "X-ObjeX-Internal-Operation"
	InternalObjectVersion     = "X-ObjeX-Object-Version"
	InternalExpectedETag      = "X-ObjeX-Expected-ETag"
	InternalObjectDeleted     = "X-ObjeX-Object-Deleted"
)

const (
	OpReplicatePut    = "replicate-put"
	OpReplicateDelete = "replicate-delete"
	OpReplicateHead   = "replicate-head"
	OpReplicateGet    = "replicate-get"
)

// IsInternalRequest reports whether the request carries a valid cluster internal token.
func IsInternalRequest(r *http.Request, expectedToken string) bool {
	if expectedToken == "" {
		return false
	}
	return r.Header.Get(InternalTokenHeader) == expectedToken
}

// InternalOperation returns the internal operation header when the request is authenticated.
func InternalOperation(r *http.Request, expectedToken string) string {
	if !IsInternalRequest(r, expectedToken) {
		return ""
	}
	return r.Header.Get(InternalOperationHeader)
}

// IsReplicatePut reports whether r is an authenticated internal replica PUT.
func IsReplicatePut(r *http.Request, expectedToken string) bool {
	return InternalOperation(r, expectedToken) == OpReplicatePut
}

// IsReplicateDelete reports whether r is an authenticated internal replica DELETE.
func IsReplicateDelete(r *http.Request, expectedToken string) bool {
	return InternalOperation(r, expectedToken) == OpReplicateDelete
}

// IsReplicateHead reports whether r is an authenticated internal replica HEAD.
func IsReplicateHead(r *http.Request, expectedToken string) bool {
	return InternalOperation(r, expectedToken) == OpReplicateHead
}

// IsReplicateGet reports whether r is an authenticated internal replica GET.
func IsReplicateGet(r *http.Request, expectedToken string) bool {
	return InternalOperation(r, expectedToken) == OpReplicateGet
}
