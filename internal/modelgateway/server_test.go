package modelgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Garden12138/xcli/internal/model"
	"github.com/Garden12138/xcli/internal/modelbackend/fake"
)

func testRegistry(t *testing.T) *model.Registry {
	t.Helper()
	r, err := model.NewRegistry([]model.Descriptor{{ID: "xcli/text-default", BackendID: "fake", UpstreamModel: "upstream", Metadata: model.Metadata{Kind: "text", Capabilities: []model.Capability{model.Text, model.Stream}, InferenceLocation: "remote"}}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestHealthAndCatalogContract(t *testing.T) {
	s, err := New(Options{Registry: testRegistry(t), Authenticate: func(token string) (Client, error) {
		if token != "fixture-token" {
			return Client{}, model.Failure(401, "invalid_api_key", "invalid")
		}
		return Client{ID: "test", AllowedModels: []string{"xcli/text-default"}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, method, host, origin, token string
		status                            int
	}{
		{"/healthz", "GET", "localhost:18741", "", "", 200},
		{"/healthz", "GET", "attacker.example:18741", "", "", 403},
		{"/healthz", "GET", "127.0.0.1:18741", "https://evil.example", "", 403},
		{"/v1/models", "GET", "127.0.0.1:18741", "", "", 401},
		{"/v1/models", "GET", "127.0.0.1:18741", "", "invalid", 401},
		{"/v1/models", "GET", "[::1]:18741", "", "fixture-token", 200},
		{"/readyz", "GET", "127.0.0.1:18741", "", "fixture-token", 200},
		{"/v1/audio/speech", "POST", "127.0.0.1:18741", "", "fixture-token", 404},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("CORS enabled")
		}
		if !json.Valid(w.Body.Bytes()) {
			t.Fatal("invalid envelope")
		}
	}
}

func TestFakeBackendIsExecutableOffline(t *testing.T) {
	b := &fake.Backend{CompleteFunc: func(ctx context.Context, r model.Object) (model.Object, error) {
		return model.Object{"choices": json.RawMessage(`[{"message":{"role":"assistant","content":"fixture"},"finish_reason":"stop","index":0}]`)}, nil
	}}
	result, err := b.Complete(context.Background(), model.Object{})
	if err != nil || !json.Valid(result["choices"]) {
		t.Fatal(result, err)
	}
	if _, err := b.Generate(context.Background(), model.Object{}); err == nil {
		t.Fatal("unimplemented fake capability returned success")
	}
	var _ http.Handler = (*Server)(nil)
}
