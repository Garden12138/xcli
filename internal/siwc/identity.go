package siwc

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

type Identity struct{ Issuer, Subject, ClientID string }

// VerifyIDToken is an offline RS256 contract using already trusted keys. G8
// deliberately has no discovery/JWKS downloader; production OIDC is still gated.
func VerifyIDToken(token Secret, keys map[string]*rsa.PublicKey, clientID, nonce, subject string, now time.Time) (Identity, error) {
	fail := func() (Identity, error) { return Identity{}, fmt.Errorf("id_token_invalid") }
	if len(token.value) > 32<<10 || nonce == "" || clientID == "" || clientID == DynamicClient {
		return fail()
	}
	p := strings.Split(token.value, ".")
	if len(p) != 3 {
		return fail()
	}
	var h struct {
		Alg  string   `json:"alg"`
		Kid  string   `json:"kid"`
		Crit []string `json:"crit"`
	}
	hb, e1 := base64.RawURLEncoding.DecodeString(p[0])
	cb, e2 := base64.RawURLEncoding.DecodeString(p[1])
	sig, e3 := base64.RawURLEncoding.DecodeString(p[2])
	if e1 != nil || e2 != nil || e3 != nil || json.Unmarshal(hb, &h) != nil || h.Alg != "RS256" || h.Kid == "" || len(h.Crit) > 0 {
		return fail()
	}
	key := keys[h.Kid]
	if key == nil || key.N == nil || key.N.BitLen() < 2048 || key.E < 3 {
		return fail()
	}
	hash := sha256.Sum256([]byte(p[0] + "." + p[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], sig) != nil {
		return fail()
	}
	var c struct {
		Iss   string          `json:"iss"`
		Sub   string          `json:"sub"`
		Aud   json.RawMessage `json:"aud"`
		Azp   string          `json:"azp"`
		Exp   int64           `json:"exp"`
		Iat   int64           `json:"iat"`
		Nbf   int64           `json:"nbf"`
		Nonce string          `json:"nonce"`
	}
	if json.Unmarshal(cb, &c) != nil || c.Iss != Issuer || c.Sub == "" || c.Exp <= now.Unix() || c.Iat <= 0 || c.Iat > now.Unix()+30 || c.Nbf > now.Unix()+30 || subtle.ConstantTimeCompare([]byte(c.Nonce), []byte(nonce)) != 1 || (subject != "" && subject != c.Sub) {
		return fail()
	}
	var aud []string
	var single string
	if json.Unmarshal(c.Aud, &single) == nil {
		aud = []string{single}
	} else if json.Unmarshal(c.Aud, &aud) != nil {
		return fail()
	}
	if !slices.Contains(aud, clientID) || (len(aud) > 1 && c.Azp != clientID) || (c.Azp != "" && c.Azp != clientID) {
		return fail()
	}
	return Identity{Issuer: c.Iss, Subject: c.Sub, ClientID: clientID}, nil
}

type Credential struct {
	Identity            Identity
	Generation          uint64
	ExpiresAt           time.Time
	EarliestRefreshAt   time.Time
	scopes              []string
	access, refresh, id Secret
}

func (Credential) Format(s fmt.State, _ rune)   { _, _ = s.Write([]byte("[SIWC credential REDACTED]")) }
func (Credential) MarshalJSON() ([]byte, error) { return []byte(`"[SIWC credential REDACTED]"`), nil }

// acceptTokenResponse uses the granted token scopes, never callback scope or an
// unverified access-token payload. Empty/missing scope cannot enable inference.
func acceptTokenResponse(data []byte, identity Identity, now time.Time) (Credential, error) {
	var r struct {
		Access   string `json:"access_token"`
		Refresh  string `json:"refresh_token"`
		ID       string `json:"id_token"`
		Type     string `json:"token_type"`
		Expires  int64  `json:"expires_in"`
		Scope    string `json:"scope"`
		Earliest int64  `json:"earliest_refresh_at"`
	}
	if len(data) > 64<<10 || json.Unmarshal(data, &r) != nil || r.Type != "Bearer" || r.Access == "" || r.Refresh == "" || r.ID == "" || r.Expires < 1 || r.Expires > 3600 || strings.ContainsAny(r.Access+r.Refresh, "\r\n") || identity.Issuer != Issuer || identity.Subject == "" || !strings.HasPrefix(identity.ClientID, "oaiapp_") {
		return Credential{}, fmt.Errorf("oauth_token_response_invalid")
	}
	return Credential{Identity: identity, Generation: 1, ExpiresAt: now.Add(time.Duration(r.Expires) * time.Second), EarliestRefreshAt: time.Unix(r.Earliest, 0), scopes: strings.Fields(r.Scope), access: Secret{r.Access}, refresh: Secret{r.Refresh}, id: Secret{r.ID}}, nil
}

// AcceptCodeResponse binds the verified ID token to this exact pending flow.
// It cannot turn an identity-only grant into plan permission.
func (a *Attempt) AcceptCodeResponse(exchange CodeExchange, data []byte, keys map[string]*rsa.PublicKey, now time.Time) (Credential, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.consumed || a.responseConsumed || exchange.verifier.value != a.verifier.value || exchange.redirect != a.redirect || !now.Before(a.expires) {
		return Credential{}, fmt.Errorf("oauth_exchange_rejected")
	}
	a.responseConsumed = true
	var wire struct {
		ID string `json:"id_token"`
	}
	if len(data) > 64<<10 || json.Unmarshal(data, &wire) != nil {
		return Credential{}, fmt.Errorf("oauth_token_response_invalid")
	}
	identity, err := VerifyIDToken(Secret{wire.ID}, keys, exchange.clientID, a.nonce.value, a.subject, now)
	if err != nil {
		return Credential{}, err
	}
	// Clear the verifier even on a subsequent store failure; the code must not
	// be exchanged again. A new attempt is required after failure.
	a.verifier = Secret{}
	return acceptTokenResponse(data, identity, now)
}

type AuthorizationStatus struct {
	SignedIn   bool   `json:"signed_in"`
	PlanUsage  bool   `json:"plan_usage"`
	Expired    bool   `json:"expired"`
	Production string `json:"production"`
}

func (c Credential) Status(now time.Time) AuthorizationStatus {
	return AuthorizationStatus{SignedIn: c.access.value != "", PlanUsage: c.access.value != "" && slices.Contains(c.scopes, "chatgpt.tokens.use.direct") && slices.Contains(c.scopes, "resource.invoke"), Expired: !now.Before(c.ExpiresAt), Production: ProductionStatus}
}
func (c Credential) RefreshForm() url.Values {
	return url.Values{"grant_type": {"refresh_token"}, "client_id": {c.Identity.ClientID}, "refresh_token": {c.refresh.value}, "resource": {Resource}}
}
func (c Credential) RevokeForm() url.Values {
	return url.Values{"token": {c.refresh.value}, "token_type_hint": {"refresh_token"}, "client_id": {c.Identity.ClientID}}
}

// Rotate is a pure replacement contract. The store must serialize the exchange
// and atomically persist next before it can be used. It must never reuse old RTs.
func (c Credential) Rotate(data []byte, now time.Time) (Credential, error) {
	// G8 retains the initially verified ID token. Refresh responses that issue
	// a different ID token require the future production OIDC refresh verifier.
	var wire map[string]json.RawMessage
	if len(data) > 64<<10 || json.Unmarshal(data, &wire) != nil || wire == nil {
		return Credential{}, fmt.Errorf("oauth_token_response_invalid")
	}
	if raw, ok := wire["id_token"]; ok {
		var id string
		if json.Unmarshal(raw, &id) != nil || id != c.id.value {
			return Credential{}, fmt.Errorf("refresh_id_validation_required")
		}
	}
	wire["id_token"] = mustJSON(c.id.value)
	next, err := acceptTokenResponse(mustJSON(wire), c.Identity, now)
	if err != nil {
		return Credential{}, err
	}
	if next.refresh.value == c.refresh.value {
		return Credential{}, fmt.Errorf("refresh_rotation_missing")
	}
	next.Generation = c.Generation + 1
	return next, nil
}
