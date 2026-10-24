package modelbackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/model"
)

func TestUpstreamErrorClassificationAndRedaction(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 503, 302} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("upstream key missing")
				}
				if r.Header.Get("X-Xcli-Remote-Consent") != "" {
					t.Error("local consent leaked")
				}
				w.Header().Set("Location", "http://127.0.0.1:1/credential-leak")
				w.WriteHeader(status)
				fmt.Fprint(w, "private upstream error prompt token")
			}))
			defer server.Close()
			b, err := NewOpenAI(server.URL+"/v1", "fixture-key")
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			_, err = b.Complete(context.Background(), model.Object{})
			var e *model.Error
			if !errors.As(err, &e) {
				t.Fatal(err)
			}
			want := status
			if status == 302 {
				want = 502
			}
			if e.Status != want || strings.Contains(e.Message, "private") {
				t.Fatal(e)
			}
		})
	}
}

func TestBoundedSSEParsing(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		ok          bool
	}{
		{"multiline", ": ping\r\ndata: {\r\ndata: \"choices\": []}\r\n\r\ndata: [DONE]\r\n\r\n", true},
		{"missing done", "data: {\"choices\":[]}\n\n", false},
		{"upstream error", "data: {\"error\":{\"message\":\"secret\"}}\n\n", false},
		{"oversize line", "data: " + strings.Repeat("a", 1<<20) + "\n\n", false},
		{"invalid UTF8", "data: {\"content\":\"\xff\"}\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			err := parseSSE(strings.NewReader(tc.input), func(o model.Object) error { count++; return nil })
			if (err == nil) != tc.ok {
				t.Fatal(err)
			}
			if tc.ok && count != 1 {
				t.Fatal(count)
			}
		})
	}
}

func TestUpstreamProtocolAndOptionalFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Error(r.URL.Path)
		}
		var req model.Object
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if string(req["temperature"]) != "0.37" || req.String("model") != "upstream" {
			t.Error(req)
		}
		fmt.Fprint(w, `{"id":"fixture","object":"chat.completion","created":1,"choices":[]}`)
	}))
	defer server.Close()
	b, err := NewOpenAI(server.URL+"/v1/", "")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_, err = b.Complete(context.Background(), model.Object{"model": json.RawMessage(`"upstream"`), "temperature": json.RawMessage(`0.37`)})
	if err != nil {
		t.Fatal(err)
	}
}
