package api

import (
	"net/http"

	"github.com/VishalPainjane/objex/internal/auth"
	"github.com/VishalPainjane/objex/internal/metrics"
	"github.com/VishalPainjane/objex/internal/s3"
)

// maybeForwardObject proxies the request to the placement primary when the local node is not responsible.
// Returns true if the response was written (forwarded or error).
func (h *Handler) maybeForwardObject(w http.ResponseWriter, r *http.Request, bucket, key, operation string) bool {
	if auth.IsInternalRequest(r, h.internalToken) {
		return false
	}
	if h.proxy == nil || h.placement == nil || h.localNodeID == "" {
		return false
	}

	result, err := h.placement.Locate(bucket, key)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("placement locate failed", "bucket", bucket, "key", key, "error", err)
		}
		s3.WriteError(w, "InternalError", "Placement calculation failed.", http.StatusInternalServerError)
		return true
	}
	if result.Primary.ID == h.localNodeID {
		return false
	}

	metrics.RecordClusterForward(operation)
	if err := h.proxy.Forward(w, r, result.Primary.Address); err != nil {
		if h.logger != nil {
			h.logger.Error("forward to primary failed",
				"bucket", bucket,
				"key", key,
				"primary", result.Primary.ID,
				"error", err,
			)
		}
		s3.WriteError(w, "InternalError", "Failed to forward request to primary node.", http.StatusInternalServerError)
	}
	return true
}
