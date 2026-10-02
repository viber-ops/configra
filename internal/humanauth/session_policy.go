package humanauth

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// SessionPolicy is shared across Management replicas. Generations never decrease;
// removing a block must not restore cookies issued before an invalidation.
type SessionPolicy struct {
	SubjectGeneration uint64
	SessionGeneration uint64
	Blocked           bool
	RevokedAt         time.Time
}

type SessionAuthority interface {
	SessionPolicy(context.Context, string, string, string) (SessionPolicy, error)
	ApplyOIDCLogout(context.Context, LogoutNotice) error
}

// LogoutNotice contains verified identifiers only, never the signed bearer token.
type LogoutNotice struct {
	Issuer, Subject, SessionID, TokenID string
	IssuedAt                            time.Time
	SourceAddr                          string
}

func (policy SessionPolicy) stamp() string {
	return strconv.FormatUint(policy.SubjectGeneration, 10) + ":" + strconv.FormatUint(policy.SessionGeneration, 10)
}
func (policy SessionPolicy) accepts(stamp string) bool {
	if policy.Blocked {
		return false
	}
	if stamp == "" {
		return policy.SubjectGeneration == 0 && policy.SessionGeneration == 0
	}
	left, right, ok := strings.Cut(stamp, ":")
	if !ok {
		return false
	}
	first, err := strconv.ParseUint(left, 10, 64)
	if err != nil {
		return false
	}
	second, err := strconv.ParseUint(right, 10, 64)
	return err == nil && first == policy.SubjectGeneration && second == policy.SessionGeneration
}
