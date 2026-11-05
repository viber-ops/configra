package humanauth

import (
	"context"
	"time"
)

type Role string

const (
	RoleViewer Role = "viewer"
	RoleAdmin  Role = "admin"
)

type Principal struct {
	Issuer      string
	Subject     string
	Email       string
	Role        Role
	MFAVerified bool
	// Session limits for the browser; zero for non-session principals.
	SessionExpiresAt   time.Time
	SessionIdleTimeout time.Duration
	SessionLifetime    time.Duration
}

func (principal Principal) ActorID() string {
	if principal.Issuer == "" {
		return principal.Subject
	}
	return principal.Issuer + "|" + principal.Subject
}

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	if !ok || principal.Subject == "" || len(principal.ActorID()) > 255 ||
		(principal.Role != RoleViewer && principal.Role != RoleAdmin) {
		return Principal{}, false
	}
	return principal, true
}
