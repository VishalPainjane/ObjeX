package auth

import "context"

type contextKey int

const credentialContextKey contextKey = 1

// ContextWithCredential stores the authenticated credential in ctx.
func ContextWithCredential(ctx context.Context, cred *Credential) context.Context {
	return context.WithValue(ctx, credentialContextKey, cred)
}

// CredentialFromContext returns the authenticated credential if present.
func CredentialFromContext(ctx context.Context) (*Credential, bool) {
	cred, ok := ctx.Value(credentialContextKey).(*Credential)
	return cred, ok && cred != nil
}
