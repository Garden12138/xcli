package siwc

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/model"
)

// All HTTP fixtures and the only network client live in _test.go. No real
// credentials, browser launch, external network, or credential files are used.
type offlineClient struct {
	base   string
	client *http.Client
	calls  atomic.Int64
}

func newOfflineClient(t *testing.T, handler http.Handler) *offlineClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return nil, fmt.Errorf("offline_non_loopback_rejected")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	t.Cleanup(transport.CloseIdleConnections)
	return &offlineClient{base: server.URL, client: &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *offlineClient) request(ctx context.Context, method, path string, form url.Values, body []byte, access Secret) (*http.Response, error) {
	var reader io.Reader = bytes.NewReader(body)
	if form != nil {
		reader = strings.NewReader(form.Encode())
	}
	r, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r.Header.Set("Content-Type", "application/json")
	}
	if access.value != "" {
		r.Header.Set("Authorization", "Bearer "+access.value)
	}
	c.calls.Add(1)
	return c.client.Do(r)
}
func readBody(t *testing.T, r *http.Response) []byte {
	t.Helper()
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func streamBody(text string) string {
	events := []any{
		map[string]any{"type": "response.created", "sequence_number": 0, "response": map[string]any{"id": "resp_synthetic", "status": "in_progress"}},
		map[string]any{"type": "response.output_text.delta", "sequence_number": 1, "delta": text},
		map[string]any{"type": "response.completed", "sequence_number": 2, "response": map[string]any{"id": "resp_synthetic", "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": text}}}}}},
	}
	var out strings.Builder
	for _, e := range events {
		fmt.Fprintf(&out, "data: %s\n\n", mustJSON(e))
	}
	return out.String()
}
func validRequest() model.Object {
	return model.Object{"model": mustJSON("synthetic-model"), "input": mustJSON([]any{map[string]any{"role": "user", "content": "synthetic question"}}), "store": mustJSON(false), "stream": mustJSON(true)}
}

func TestOfflineOAuthResponsesLifecycle(t *testing.T) {
	k := newKey(t)
	a := newAttempt(t, "", "")
	id := signed(t, k, claims(a, "oaiapp_synthetic"))
	var granted atomic.Bool
	var generation atomic.Int64
	generation.Store(1)
	client := newOfflineClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorize":
			q := r.URL.Query()
			hash := sha256.Sum256([]byte(a.verifier.value))
			if q.Get("client_id") != DynamicClient || q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(hash[:]) || q.Get("scope") != Scopes {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Location", callback(a, "oaiapp_synthetic"))
			w.WriteHeader(http.StatusFound)
		case "/token":
			_ = r.ParseForm()
			if r.Method != "POST" || r.Form.Get("client_id") != "oaiapp_synthetic" || r.Form.Get("resource") != Resource || r.Form.Has("client_secret") {
				w.WriteHeader(400)
				return
			}
			switch r.Form.Get("grant_type") {
			case "authorization_code":
				if r.Form.Get("code") != "synthetic-code" || r.Form.Get("code_verifier") != a.verifier.value || r.Form.Get("redirect_uri") != a.redirect {
					w.WriteHeader(400)
					return
				}
				granted.Store(true)
			case "refresh_token":
				if r.Form.Get("refresh_token") != "synthetic-refresh-1" || r.Form.Has("scope") {
					w.WriteHeader(400)
					return
				}
				generation.Store(2)
			default:
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(tokenData(id, Scopes, int(generation.Load())))
		case "/revoke":
			_ = r.ParseForm()
			if r.Method != "POST" || r.Form.Get("token") != "synthetic-refresh-2" || r.Form.Get("client_id") != "oaiapp_synthetic" || r.Form.Get("token_type_hint") != "refresh_token" {
				w.WriteHeader(400)
				return
			}
			granted.Store(false)
		case "/v1/models", "/v1/responses":
			if !granted.Load() || r.Header.Get("Authorization") != fmt.Sprintf("Bearer synthetic-access-%d", generation.Load()) {
				w.WriteHeader(401)
				return
			}
			if r.URL.Path == "/v1/models" {
				_, _ = io.WriteString(w, `{"models":[{"slug":"synthetic-model","display_name":"Synthetic Model","visibility":"list"},{"slug":"hidden","visibility":"hide"}]}`)
				return
			}
			var req model.Object
			if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&req) != nil || ValidateRequest(req) != nil {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for _, b := range []byte(streamBody("synthetic result")) {
				_, _ = w.Write([]byte{b})
			}
			_ = http.NewResponseController(w).Flush()
		default:
			w.WriteHeader(404)
		}
	}))
	ctx := context.Background()
	authURL, _ := url.Parse(a.AuthorizationURL())
	res, err := client.request(ctx, "GET", "/authorize?"+authURL.RawQuery, nil, nil, Secret{})
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 302 {
		t.Fatal("mock browser authorization failed")
	}
	exchange, err := a.Callback(res.Header.Get("Location"), testNow)
	if err != nil {
		t.Fatal(err)
	}
	res, err = client.request(ctx, "POST", "/token", exchange.Form(), nil, Secret{})
	if err != nil || res.StatusCode != 200 {
		t.Fatal("mock code exchange", err)
	}
	credential, err := a.AcceptCodeResponse(exchange, readBody(t, res), map[string]*rsa.PublicKey{"test-key": &k.PublicKey}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	store := newMemoryStore()
	if err = store.Replace("registration", 0, credential); err != nil {
		t.Fatal(err)
	}
	res, err = client.request(ctx, "GET", "/v1/models", nil, nil, credential.access)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ParseCatalog(readBody(t, res))
	if err != nil || len(catalog) != 1 || catalog[0].InferenceVerified {
		t.Fatal("catalog is not entitlement proof", err)
	}
	res, err = client.request(ctx, "POST", "/v1/responses", nil, mustJSON(validRequest()), credential.access)
	if err != nil {
		t.Fatal(err)
	}
	var terminal Event
	err = ConsumeStream(ctx, res.Body, func(e Event) error {
		terminal = e
		data, _ := json.Marshal(e)
		if bytes.Contains(data, []byte("synthetic-access")) {
			t.Fatal("token delivered to app")
		}
		return nil
	})
	_ = res.Body.Close()
	if err != nil || terminal.Type != "response.completed" || terminal.Text != "synthetic result" {
		t.Fatal("Responses lifecycle", err)
	}
	history := NewHistory()
	owner := HistoryOwner{"worklog", "registration", "conversation"}
	var input []model.Object
	_ = json.Unmarshal(validRequest()["input"], &input)
	if err = history.Commit(owner, input, terminal, testNow); err != nil {
		t.Fatal(err)
	}
	res, err = client.request(ctx, "POST", "/token", credential.RefreshForm(), nil, Secret{})
	if err != nil {
		t.Fatal(err)
	}
	next, err := credential.Rotate(readBody(t, res), testNow.Add(50*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Replace("registration", 1, next); err != nil {
		t.Fatal(err)
	}
	res, err = client.request(ctx, "POST", "/v1/responses", nil, mustJSON(validRequest()), credential.access)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("rotated old token accepted")
	}
	res, err = client.request(ctx, "POST", "/revoke", next.RevokeForm(), nil, Secret{})
	if err != nil || res.StatusCode != 200 || len(readBody(t, res)) != 0 {
		t.Fatal("revoke", err)
	}
	_ = store.Clear("registration")
	history.ClearRegistration("registration")
	if len(history.Load(owner, testNow)) != 0 {
		t.Fatal("logout retained history")
	}
	res, err = client.request(ctx, "POST", "/v1/responses", nil, mustJSON(validRequest()), next.access)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("revoked token accepted")
	}
	if client.calls.Load() != 8 {
		t.Fatalf("unexpected simulated request count: %d", client.calls.Load())
	}
}

func TestOfflineTimeoutCancellationAndRedirect(t *testing.T) {
	client := newOfflineClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "https://api.openai.com/v1/responses")
			w.WriteHeader(307)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Split(streamBody("x"), "\n\n")[0]+"\n\n")
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
	}))
	for _, deadline := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		if deadline {
			ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
		}
		res, err := client.request(ctx, "POST", "/slow", nil, mustJSON(validRequest()), Secret{"synthetic-access"})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if !deadline {
			cancel()
		}
		err = ConsumeStream(ctx, res.Body, func(Event) error { return nil })
		_ = res.Body.Close()
		cancel()
		if err != context.Canceled && err != context.DeadlineExceeded {
			t.Fatalf("cancellation classification: %T", err)
		}
	}
	res, err := client.request(context.Background(), "POST", "/redirect", nil, nil, Secret{"synthetic-access"})
	if err != nil || res.StatusCode != 307 {
		t.Fatal("redirect followed", err)
	}
	_ = res.Body.Close()
	r, _ := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	if _, err = client.client.Do(r); err == nil {
		t.Fatal("offline client escaped loopback")
	}
}

func TestOfflineDeltaArrivesBeforeCompletion(t *testing.T) {
	observed := make(chan struct{})
	client := newOfflineClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(streamBody("progressive text"), "\n\n")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, parts[0]+"\n\n"+parts[1]+"\n\n")
		_ = http.NewResponseController(w).Flush()
		select {
		case <-observed:
			_, _ = io.WriteString(w, parts[2]+"\n\n")
		case <-r.Context().Done():
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := client.request(ctx, "POST", "/v1/responses", nil, mustJSON(validRequest()), Secret{"synthetic-access"})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	completed := false
	err = ConsumeStream(ctx, res.Body, func(e Event) error {
		if e.Type == "response.output_text.delta" {
			close(observed)
		}
		if e.Type == "response.completed" {
			completed = true
		}
		return nil
	})
	if err != nil || !completed {
		t.Fatal("delta was not delivered before server completion", err)
	}
}
