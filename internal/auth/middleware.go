package auth

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/VishalPainjane/objex/internal/s3"
)

// Middleware enforces SigV4 authentication on S3 API routes.
type Middleware struct {
	Verifier       *Verifier
	Logger         *slog.Logger
	Skip           func(r *http.Request) bool
	InternalToken  string
}

// Handler wraps next with SigV4 verification.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.Skip != nil && m.Skip(r) {
			next.ServeHTTP(w, r)
			return
		}

		if IsInternalRequest(r, m.InternalToken) {
			next.ServeHTTP(w, r)
			return
		}

		cred, err := m.Verifier.VerifyRequest(r.Context(), r)
		if err != nil {
			if ae, ok := IsAuthError(err); ok {
				s3.WriteError(w, ae.Code, ae.Message, ae.StatusCode)
				return
			}
			if m.Logger != nil {
				m.Logger.Error("auth internal error", "error", err)
			}
			s3.WriteError(w, "InternalError", "An internal error occurred.", http.StatusInternalServerError)
			return
		}
		r = r.WithContext(ContextWithCredential(r.Context(), cred))
		next.ServeHTTP(w, r)
	})
}

// DefaultSkip returns true for health and metrics paths.
func DefaultSkip(r *http.Request) bool {
	path := r.URL.Path
	if path == "/health" || path == "/health/live" || path == "/health/ready" || path == "/metrics" || path == "/cluster" || path == "/debug/placement" {
		return true
	}
	return false
}

// SanitizeAccessKeyID removes control characters for safe logging.
func SanitizeAccessKeyID(id string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(id)
}
