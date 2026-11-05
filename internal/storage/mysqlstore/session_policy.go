package mysqlstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/viber-ops/configra/internal/humanauth"
)

type SessionChange struct {
	OperationID    string
	Actor          Actor
	Issuer         string `json:"issuer"`
	Subject        string `json:"subject"`
	SessionID      string `json:"-"`
	Action         string `json:"action"` // invalidate, block, unblock
	logoutIssuedAt time.Time
}
type SessionChangeResult struct {
	Outcome    Outcome `json:"outcome"`
	IdentityID string  `json:"identity_id"`
	Generation uint64  `json:"generation"`
	Blocked    bool    `json:"blocked"`
}
type SessionPolicySummary struct {
	IdentityID string    `json:"identity_id"`
	Issuer     string    `json:"issuer"`
	Subject    string    `json:"subject"`
	Generation uint64    `json:"generation"`
	Blocked    bool      `json:"blocked"`
	RevokedAt  time.Time `json:"revoked_at"`
}

func sessionIdentity(kind, issuer, target string) [32]byte {
	encoded, _ := json.Marshal([3]string{kind, issuer, target})
	return sha256.Sum256(encoded)
}

func (store *Store) SessionPolicy(ctx context.Context, issuer, subject, sid string) (humanauth.SessionPolicy, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var policy humanauth.SessionPolicy
	for _, part := range []struct {
		kind, target string
		version      *uint64
	}{{"subject", subject, &policy.SubjectGeneration}, {"sid", sid, &policy.SessionGeneration}} {
		if part.target == "" {
			continue
		}
		id := sessionIdentity(part.kind, issuer, part.target)
		var blocked bool
		var revoked time.Time
		err := store.db.QueryRowContext(ctx, `SELECT generation,blocked,revoked_at FROM management_session_policies WHERE identity_id=?`, id[:]).Scan(part.version, &blocked, &revoked)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return policy, errors.New("read session policy")
		}
		policy.Blocked = policy.Blocked || blocked
		if revoked.After(policy.RevokedAt) {
			policy.RevokedAt = revoked.UTC()
		}
	}
	return policy, nil
}

func (store *Store) ChangeSessions(ctx context.Context, request SessionChange) (SessionChangeResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) || request.Issuer == "" || len(request.Issuer) > 512 || (request.Subject == "") == (request.SessionID == "") || len(request.Subject) > 255 || len(request.SessionID) > 1024 || (request.Action != "invalidate" && request.Action != "block" && request.Action != "unblock") {
		return SessionChangeResult{}, ErrValidation
	}
	kind, target := "subject", request.Subject
	if request.SessionID != "" {
		kind, target = "sid", request.SessionID
	}
	id := sessionIdentity(kind, request.Issuer, target)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionChangeResult{}, errors.New("begin session change")
	}
	defer tx.Rollback()
	if err := store.lockMasterKey(ctx, tx, false); err != nil {
		return SessionChangeResult{}, err
	}
	replayed, replay, err := beginOperation(ctx, tx, request.OperationID, tokenRequestDigest(struct {
		Request  SessionChange
		SID      string
		IssuedAt time.Time
	}{request, request.SessionID, request.logoutIssuedAt}), request.Actor)
	if err != nil {
		return SessionChangeResult{}, err
	}
	if replayed {
		var previous SessionChangeResult
		if json.Unmarshal(replay.Response, &previous) != nil {
			return previous, errors.New("invalid session replay")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO management_session_policies(identity_id,issuer,target_kind,target,generation,blocked,revoked_at) VALUES(?,?,?,?,1,?,UTC_TIMESTAMP(6))
		ON DUPLICATE KEY UPDATE generation=generation+1,blocked=IF(?='invalidate',blocked,VALUES(blocked)),revoked_at=UTC_TIMESTAMP(6)`, id[:], request.Issuer, kind, target, request.Action == "block", request.Action); err != nil {
		return SessionChangeResult{}, errors.New("change session policy")
	}
	result := SessionChangeResult{Outcome: OutcomeSuccess, IdentityID: hex.EncodeToString(id[:])}
	if err := tx.QueryRowContext(ctx, `SELECT generation,blocked FROM management_session_policies WHERE identity_id=?`, id[:]).Scan(&result.Generation, &result.Blocked); err != nil {
		return result, errors.New("read session change")
	}
	if err := finishOperation(ctx, tx, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{Type: "identity." + request.Action, Action: "identity." + request.Action, ResourceType: "identity", ResourceKey: result.IdentityID}, false); err != nil {
		return result, err
	}
	if tx.Commit() != nil {
		return SessionChangeResult{}, errors.New("commit session change")
	}
	return result, nil
}

func (store *Store) ApplyOIDCLogout(ctx context.Context, notice humanauth.LogoutNotice) error {
	if notice.TokenID == "" || len(notice.TokenID) > 1024 || notice.IssuedAt.IsZero() {
		return ErrValidation
	}
	key := sessionIdentity("logout", notice.Issuer, notice.TokenID)
	change := SessionChange{OperationID: "oidc-logout-" + hex.EncodeToString(key[:]), Actor: Actor{Type: "system", ID: "oidc-backchannel-logout"}, Issuer: notice.Issuer, Subject: notice.Subject, Action: "invalidate", logoutIssuedAt: notice.IssuedAt}
	if notice.SessionID != "" {
		change.Subject = ""
		change.SessionID = notice.SessionID
	}
	_, err := store.ChangeSessions(WithAuditSourceIP(ctx, notice.SourceAddr), change)
	return err
}

func (store *Store) ListSessionPolicies(ctx context.Context, query InventoryQuery) (InventoryPage[SessionPolicySummary], error) {
	page := InventoryPage[SessionPolicySummary]{Items: make([]SessionPolicySummary, 0)}
	if !query.valid() || query.Key != "" || query.IncludeInactive || query.OnlyInactive {
		return page, ErrValidation
	}
	const filter = " FROM management_session_policies WHERE target_kind='subject' AND LOCATE(LOWER(?),LOWER(CONCAT_WS(' ',issuer,target)))>0"
	if store.db.QueryRowContext(ctx, "SELECT COUNT(*)"+filter, query.Search).Scan(&page.Total) != nil {
		return page, errors.New("count session policies")
	}
	rows, err := store.db.QueryContext(ctx, "SELECT LOWER(HEX(identity_id)),issuer,target,generation,blocked,revoked_at"+filter+" ORDER BY identity_id LIMIT ? OFFSET ?", query.Search, query.Limit, query.Offset)
	if err != nil {
		return page, errors.New("list session policies")
	}
	defer rows.Close()
	for rows.Next() {
		var entry SessionPolicySummary
		if rows.Scan(&entry.IdentityID, &entry.Issuer, &entry.Subject, &entry.Generation, &entry.Blocked, &entry.RevokedAt) != nil {
			return page, errors.New("read session policy metadata")
		}
		page.Items = append(page.Items, entry)
	}
	if rows.Err() != nil {
		return page, errors.New("read session policies")
	}
	return page, nil
}
