package modelgateway

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/model"
)

func TestG8DoesNotExposeOAuthOrEnableResponsesRoute(t *testing.T) {
	s, err := New(Options{Registry: testRegistry(t), Authenticate: func(token string) (Client, error) {
		if token != "local-app-token" {
			return Client{}, model.Failure(401, "invalid_api_key", "invalid")
		}
		return Client{ID: "worklog", AllowedModels: []string{"xcli/text-default"}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/responses", "/siwc/token", "/siwc/login", "/siwc/refresh", "/siwc/accounts"} {
		for _, token := range []string{"local-app-token", "synthetic-oauth-access-token", ""} {
			r := httptest.NewRequest("POST", "http://127.0.0.1:18741"+path, strings.NewReader(`{}`))
			if token != "" {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			want := 401
			if token == "local-app-token" {
				want = 404
			}
			if w.Code != want {
				t.Fatal(path, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), token) && token != "" {
				t.Fatal("request credential echoed")
			}
		}
	}
}
