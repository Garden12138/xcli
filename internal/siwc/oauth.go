package siwc

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var hostPattern = regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// NewHostID generates the supported UUIDv4 alternative. The caller must persist
// it once per installation before authorization; restarting is not a new host.
func NewHostID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6], b[8] = (b[6]&15)|64, (b[8]&63)|128
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

type Attempt struct {
	mu                                  sync.Mutex
	clientID, subject, hostID, redirect string
	state, nonce, verifier              Secret
	expires                             time.Time
	consumed                            bool
	responseConsumed                    bool
}

func (*Attempt) Format(s fmt.State, _ rune)   { _, _ = s.Write([]byte("[OAuth attempt REDACTED]")) }
func (*Attempt) MarshalJSON() ([]byte, error) { return []byte(`"[OAuth attempt REDACTED]"`), nil }

func NewAttempt(hostID, redirect, clientID, subject string, now time.Time) (*Attempt, error) {
	u, err := url.Parse(redirect)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "/auth/callback" || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || !hostPattern.MatchString(hostID) {
		return nil, fmt.Errorf("invalid SIWC host or callback")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid SIWC callback port")
	}
	if clientID == "" {
		clientID = DynamicClient
	}
	if clientID != DynamicClient && (!strings.HasPrefix(clientID, "oaiapp_") || subject == "") {
		return nil, fmt.Errorf("returning registration needs issued client and verified subject")
	}
	a := &Attempt{clientID: clientID, subject: subject, hostID: hostID, redirect: redirect, expires: now.Add(5 * time.Minute)}
	for _, secret := range []*Secret{&a.state, &a.nonce, &a.verifier} {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		secret.value = base64.RawURLEncoding.EncodeToString(b)
	}
	return a, nil
}

// AuthorizationURL is for a trusted browser launcher only, never diagnostics or
// business clients. G8 does not open it or bind a real callback listener.
func (a *Attempt) AuthorizationURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	hash := sha256.Sum256([]byte(a.verifier.value))
	q := url.Values{"client_id": {a.clientID}, "ext_agent_host_id": {a.hostID}, "response_type": {"code"}, "redirect_uri": {a.redirect}, "scope": {Scopes}, "resource": {Resource}, "state": {a.state.value}, "nonce": {a.nonce.value}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}}
	if a.clientID == DynamicClient {
		q.Set("agent_name_hint", "xcli")
	}
	return AuthorizeEndpoint + "?" + q.Encode()
}

type CodeExchange struct {
	clientID       string
	code, verifier Secret
	redirect       string
}

func (CodeExchange) Format(s fmt.State, _ rune) { _, _ = s.Write([]byte("[code exchange REDACTED]")) }

// Callback checks the exact URI, parameter cardinality, state, expiration and
// returning client binding before releasing a one-shot exchange description.
func (a *Attempt) Callback(callback string, now time.Time) (CodeExchange, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fail := func() (CodeExchange, error) { return CodeExchange{}, fmt.Errorf("oauth_callback_rejected") }
	u, err := url.Parse(callback)
	if err != nil || len(callback) > 16<<10 || u.User != nil || u.Fragment != "" || u.Scheme+"://"+u.Host+u.EscapedPath() != a.redirect || a.consumed || !now.Before(a.expires) {
		return fail()
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return fail()
	}
	for _, v := range q {
		if len(v) != 1 {
			return fail()
		}
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(a.state.value)) != 1 {
		return fail()
	}
	a.consumed = true
	if q.Get("error") != "" || q.Get("code") == "" {
		return fail()
	}
	id := q.Get("client_id")
	if a.clientID == DynamicClient {
		if !strings.HasPrefix(id, "oaiapp_") || len(id) <= len("oaiapp_") {
			return fail()
		}
	} else {
		if id != "" && id != a.clientID {
			return fail()
		}
		id = a.clientID
	}
	return CodeExchange{clientID: id, code: Secret{q.Get("code")}, verifier: a.verifier, redirect: a.redirect}, nil
}

func (e CodeExchange) Form() url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "client_id": {e.clientID}, "code": {e.code.value}, "code_verifier": {e.verifier.value}, "redirect_uri": {e.redirect}, "resource": {Resource}}
}
