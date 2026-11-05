package humanauth

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const logoutEvent = "http://schemas.openid.net/event/backchannel-logout"

func (authentication *OIDC) backchannelLogout(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Cache-Control", "no-store")
	if authentication.authority == nil {
		writeOIDCError(response, 503, "authentication_unavailable")
		return
	}
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		writeOIDCError(response, 400, "invalid_logout_token")
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 16<<10)
	if request.ParseForm() != nil || len(request.PostForm["logout_token"]) != 1 {
		writeOIDCError(response, 400, "invalid_logout_token")
		return
	}
	encoded := request.PostForm.Get("logout_token")
	token, err := authentication.logoutVerifier.Verify(request.Context(), encoded)
	if err != nil {
		writeOIDCError(response, 400, "invalid_logout_token")
		return
	}
	var claims map[string]json.RawMessage
	if token.Claims(&claims) != nil {
		writeOIDCError(response, 400, "invalid_logout_token")
		return
	}
	var events map[string]json.RawMessage
	if json.Unmarshal(claims["events"], &events) != nil {
		writeOIDCError(response, 400, "invalid_logout_token")
		return
	}
	var event map[string]json.RawMessage
	if json.Unmarshal(events[logoutEvent], &event) != nil || event == nil || len(event) != 0 {
		writeOIDCError(response, 400, "invalid_logout_token")
		return
	}
	_, hasNonce := claims["nonce"]
	_, hasExpiry := claims["exp"]
	sid, jti := claimString(claims, "sid"), claimString(claims, "jti")
	now := authentication.now()
	if encoded, present := claims["nbf"]; present {
		var before int64
		if json.Unmarshal(encoded, &before) != nil || time.Unix(before, 0).After(now) {
			writeOIDCError(response, 400, "invalid_logout_token")
			return
		}
	}
	if hasNonce || jti == "" || len(jti) > 1024 || (token.Subject == "" && sid == "") || len(token.Subject) > 255 || len(sid) > 1024 || token.IssuedAt.IsZero() || token.IssuedAt.After(now.Add(time.Minute)) || token.IssuedAt.Before(now.Add(-10*time.Minute)) || (hasExpiry && !now.Before(token.Expiry)) {
		writeOIDCError(response, 400, "invalid_logout_token")
		return
	}
	if err := authentication.authority.ApplyOIDCLogout(request.Context(), LogoutNotice{Issuer: token.Issuer, Subject: token.Subject, SessionID: sid, TokenID: jti, IssuedAt: token.IssuedAt, SourceAddr: request.RemoteAddr}); err != nil {
		writeOIDCError(response, 503, "authentication_unavailable")
		return
	}
	response.WriteHeader(http.StatusOK)
}
