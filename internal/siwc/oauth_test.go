package siwc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func newAttempt(t *testing.T, client, subject string) *Attempt {
	t.Helper()
	host, err := NewHostID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAttempt(host, "http://127.0.0.1:1455/auth/callback", client, subject, testNow)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}
func claims(a *Attempt, client string) map[string]any {
	return map[string]any{"iss": Issuer, "sub": "subject-test", "aud": client, "iat": testNow.Unix(), "exp": testNow.Add(time.Hour).Unix(), "nonce": a.nonce.value}
}
func sign(t *testing.T, k *rsa.PrivateKey, h, c any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString(mustJSON(h))
	body := base64.RawURLEncoding.EncodeToString(mustJSON(c))
	hash := sha256.Sum256([]byte(header + "." + body))
	sig, err := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return header + "." + body + "." + base64.RawURLEncoding.EncodeToString(sig)
}
func signed(t *testing.T, k *rsa.PrivateKey, c any) string {
	return sign(t, k, map[string]any{"alg": "RS256", "kid": "test-key"}, c)
}
func tokenData(id, scope string, generation int) []byte {
	return mustJSON(map[string]any{"access_token": fmt.Sprintf("synthetic-access-%d", generation), "refresh_token": fmt.Sprintf("synthetic-refresh-%d", generation), "id_token": id, "token_type": "Bearer", "expires_in": 3600, "scope": scope, "earliest_refresh_at": testNow.Add(30 * time.Minute).Unix()})
}
func callback(a *Attempt, client string) string {
	q := url.Values{"code": {"synthetic-code"}, "state": {a.state.value}}
	if client != "" {
		q.Set("client_id", client)
	}
	return a.redirect + "?" + q.Encode()
}

func TestOAuthPKCECallbackAndReplay(t *testing.T) {
	a := newAttempt(t, "", "")
	b := newAttempt(t, "", "")
	u, _ := url.Parse(a.AuthorizationURL())
	q := u.Query()
	hash := sha256.Sum256([]byte(a.verifier.value))
	if q.Get("client_id") != DynamicClient || q.Get("agent_name_hint") != "xcli" || q.Get("scope") != Scopes || q.Get("resource") != Resource || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(hash[:]) || len(a.verifier.value) != 43 || a.state.value == b.state.value || a.nonce.value == b.nonce.value || !hostPattern.MatchString(a.hostID) {
		t.Fatal("authorization contract")
	}
	for _, port := range []string{"0", "65536", "invalid"} {
		if _, err := NewAttempt(a.hostID, "http://127.0.0.1:"+port+"/auth/callback", "", "", testNow); err == nil {
			t.Fatal("invalid callback port accepted")
		}
	}
	for name, mutate := range map[string]func(*Attempt) string{
		"state":              func(a *Attempt) string { return strings.Replace(callback(a, "oaiapp_new"), a.state.value, "wrong", 1) },
		"duplicate":          func(a *Attempt) string { return callback(a, "oaiapp_new") + "&state=duplicate" },
		"missing-client":     func(a *Attempt) string { return callback(a, "") },
		"placeholder-client": func(a *Attempt) string { return callback(a, DynamicClient) },
		"path": func(a *Attempt) string {
			return strings.Replace(callback(a, "oaiapp_new"), "/auth/callback", "/callback", 1)
		},
		"host": func(a *Attempt) string {
			return strings.Replace(callback(a, "oaiapp_new"), "127.0.0.1", "localhost", 1)
		},
		"port":   func(a *Attempt) string { return strings.Replace(callback(a, "oaiapp_new"), "1455", "1456", 1) },
		"denied": func(a *Attempt) string { return callback(a, "oaiapp_new") + "&error=access_denied" },
	} {
		t.Run(name, func(t *testing.T) {
			a := newAttempt(t, "", "")
			if _, err := a.Callback(mutate(a), testNow); err == nil {
				t.Fatal("callback accepted")
			}
		})
	}
	if _, err := a.Callback(callback(a, "oaiapp_new"), testNow.Add(5*time.Minute)); err == nil {
		t.Fatal("expired callback")
	}
	e, err := a.Callback(callback(a, "oaiapp_new"), testNow)
	if err != nil {
		t.Fatal(err)
	}
	f := e.Form()
	if f.Get("client_id") != "oaiapp_new" || f.Get("code_verifier") != a.verifier.value || f.Get("redirect_uri") != a.redirect || f.Get("resource") != Resource || f.Get("client_secret") != "" {
		t.Fatal("exchange contract")
	}
	if _, err = a.Callback(callback(a, "oaiapp_new"), testNow); err == nil {
		t.Fatal("replayed callback")
	}
	returning := newAttempt(t, "oaiapp_saved", "subject-test")
	u, _ = url.Parse(returning.AuthorizationURL())
	if u.Query().Has("agent_name_hint") {
		t.Fatal("returning name hint")
	}
	if _, err = returning.Callback(callback(returning, "oaiapp_swapped"), testNow); err == nil {
		t.Fatal("registration substitution")
	}
	returning = newAttempt(t, "oaiapp_saved", "subject-test")
	e, err = returning.Callback(callback(returning, ""), testNow)
	if err != nil || e.clientID != "oaiapp_saved" {
		t.Fatal("omitted returning client")
	}
}

func TestIDTokenAndAccountScopeBinding(t *testing.T) {
	k := newKey(t)
	keys := map[string]*rsa.PublicKey{"test-key": &k.PublicKey}
	a := newAttempt(t, "", "")
	for name, change := range map[string]func(map[string]any){
		"issuer":               func(c map[string]any) { c["iss"] = "https://attacker.invalid" },
		"audience":             func(c map[string]any) { c["aud"] = "oaiapp_other" },
		"expired":              func(c map[string]any) { c["exp"] = testNow.Unix() },
		"future-iat":           func(c map[string]any) { c["iat"] = testNow.Add(time.Hour).Unix() },
		"future-nbf":           func(c map[string]any) { c["nbf"] = testNow.Add(time.Hour).Unix() },
		"nonce":                func(c map[string]any) { c["nonce"] = "wrong" },
		"missing-sub":          func(c map[string]any) { delete(c, "sub") },
		"account-substitution": func(c map[string]any) { c["sub"] = "different-subject" },
		"ambiguous-audience":   func(c map[string]any) { c["aud"] = []string{"oaiapp_test", "other"} },
	} {
		t.Run(name, func(t *testing.T) {
			c := claims(a, "oaiapp_test")
			change(c)
			if _, err := VerifyIDToken(Secret{signed(t, k, c)}, keys, "oaiapp_test", a.nonce.value, "subject-test", testNow); err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
	for _, header := range []any{map[string]any{"alg": "none", "kid": "test-key"}, map[string]any{"alg": "HS256", "kid": "test-key"}, map[string]any{"alg": "RS256", "kid": "unknown"}, map[string]any{"alg": "RS256", "kid": "test-key", "crit": []string{"unhandled"}}} {
		if _, err := VerifyIDToken(Secret{sign(t, k, header, claims(a, "oaiapp_test"))}, keys, "oaiapp_test", a.nonce.value, "", testNow); err == nil {
			t.Fatal("invalid JWT header")
		}
	}
	other := newKey(t)
	if _, err := VerifyIDToken(Secret{signed(t, other, claims(a, "oaiapp_test"))}, keys, "oaiapp_test", a.nonce.value, "", testNow); err == nil {
		t.Fatal("signature not checked")
	}
	if _, err := VerifyIDToken(Secret{signed(t, k, claims(a, "oaiapp_test"))}, map[string]*rsa.PublicKey{"test-key": {}}, "oaiapp_test", a.nonce.value, "", testNow); err == nil {
		t.Fatal("malformed trusted key accepted")
	}
	failed := newAttempt(t, "", "")
	exchange, _ := failed.Callback(callback(failed, "oaiapp_test"), testNow)
	if _, err := failed.AcceptCodeResponse(exchange, tokenData("invalid", Scopes, 1), keys, testNow); err == nil {
		t.Fatal("invalid code response accepted")
	}
	if _, err := failed.AcceptCodeResponse(exchange, tokenData(signed(t, k, claims(failed, "oaiapp_test")), Scopes, 1), keys, testNow); err == nil {
		t.Fatal("failed response attempt reused")
	}
	for _, scope := range []string{Scopes, "openid profile email", "openid chatgpt.tokens.use.direct"} {
		a := newAttempt(t, "", "")
		id := signed(t, k, claims(a, "oaiapp_test"))
		e, err := a.Callback(callback(a, "oaiapp_test"), testNow)
		if err != nil {
			t.Fatal(err)
		}
		c, err := a.AcceptCodeResponse(e, tokenData(id, scope, 1), keys, testNow)
		if err != nil {
			t.Fatal(err)
		}
		if c.Status(testNow).PlanUsage != (scope == Scopes) {
			t.Fatal("identity grant treated as plan consent")
		}
		if _, err = a.AcceptCodeResponse(e, tokenData(id, scope, 1), keys, testNow); err == nil {
			t.Fatal("replayed response")
		}
	}
}

// This store exists only in the test binary; it is never a fallback for Keychain.
type memoryStore struct {
	mu      sync.Mutex
	records map[string]Credential
}

func newMemoryStore() *memoryStore { return &memoryStore{records: map[string]Credential{}} }
func (s *memoryStore) Load(key string) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.records[key]
	if !ok {
		return Credential{}, errors.New("signed_out")
	}
	return v, nil
}
func (s *memoryStore) Replace(key string, g uint64, next Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records[key].Generation != g || next.Generation != g+1 {
		return errors.New("stale_generation")
	}
	s.records[key] = next
	return nil
}
func (s *memoryStore) Clear(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
	return nil
}

func TestCredentialRotationRedactionAndFailClosed(t *testing.T) {
	k := newKey(t)
	a := newAttempt(t, "", "")
	id := signed(t, k, claims(a, "oaiapp_test"))
	e, _ := a.Callback(callback(a, "oaiapp_test"), testNow)
	c, err := a.AcceptCodeResponse(e, tokenData(id, Scopes, 1), map[string]*rsa.PublicKey{"test-key": &k.PublicKey}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	store := newMemoryStore()
	if err = store.Replace("registration", 0, c); err != nil {
		t.Fatal(err)
	}
	form := c.RefreshForm()
	if form.Get("client_id") != "oaiapp_test" || form.Has("scope") || form.Get("resource") != Resource || form.Get("refresh_token") != "synthetic-refresh-1" {
		t.Fatal("refresh contract")
	}
	next, err := c.Rotate(tokenData(id, Scopes, 2), testNow.Add(50*time.Minute))
	if err != nil || next.Generation != 2 {
		t.Fatal("rotation", err)
	}
	downgraded, err := c.Rotate(tokenData(id, "openid profile email", 2), testNow.Add(50*time.Minute))
	if err != nil || downgraded.Status(testNow).PlanUsage {
		t.Fatal("refresh scope downgrade ignored")
	}
	withoutID := map[string]json.RawMessage{}
	_ = json.Unmarshal(tokenData(id, Scopes, 2), &withoutID)
	delete(withoutID, "id_token")
	if _, err := c.Rotate(mustJSON(withoutID), testNow.Add(50*time.Minute)); err != nil {
		t.Fatal("omitted refresh ID token", err)
	}
	if _, err := c.Rotate(tokenData("new-unverified-id", Scopes, 2), testNow); err == nil {
		t.Fatal("unverified refresh identity accepted")
	}
	if err = store.Replace("registration", 1, next); err != nil {
		t.Fatal(err)
	}
	if err = store.Replace("registration", 1, next); err == nil {
		t.Fatal("stale writer overwrote rotation")
	}
	if _, err = c.Rotate(tokenData(id, Scopes, 1), testNow); err == nil {
		t.Fatal("reused RT")
	}
	if !c.Status(testNow.Add(time.Hour)).Expired {
		t.Fatal("expiry")
	}
	for _, v := range []any{c, c.access, a, e} {
		jsonData, _ := json.Marshal(v)
		formatted := fmt.Sprintf("%v %+v %#v %s", v, v, v, v) + string(jsonData)
		for _, secret := range []string{c.access.value, c.refresh.value, id, e.code.value, e.verifier.value} {
			if secret != "" && strings.Contains(formatted, secret) {
				t.Fatal("secret leaked via formatting")
			}
		}
	}
	if s, err := OpenSystemStore(); s != nil || !errors.Is(err, ErrSecureStoreUnavailable) {
		t.Fatal("unsafe secure-store fallback")
	}
	if c.RevokeForm().Get("token_type_hint") != "refresh_token" {
		t.Fatal("revoke contract")
	}
	_ = store.Clear("registration")
	if _, err = store.Load("registration"); err == nil {
		t.Fatal("logout retained credentials")
	}
}
