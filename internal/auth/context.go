package auth

import "context"

// Claims live in the request context under a key owned by this package. The
// middleware package imports auth (to know the Claims type), so the key and its
// accessors must live here — not in middleware — or auth could not read the
// session it needs for audit scoping without an import cycle.
type ctxKey int

const claimsKey ctxKey = 1

// WithClaims returns a context carrying the session claims. The auth middleware
// calls this after verifying a token.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ClaimsFromContext returns the session claims, or nil when none are present.
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}
