package modelgateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/model"
	"github.com/Garden12138/xcli/internal/modelbackend"
	"github.com/Garden12138/xcli/internal/modelbackend/fake"
)

const chatBody = `{"model":"xcli/text-default","messages":[{"role":"system","content":"中文 fixture"},{"role":"user","content":"synthetic"}],"temperature":0.3}`
const completionBody = `{"id":"chatcmpl-fixture","object":"chat.completion","created":1,"model":"upstream","choices":[{"index":0,"message":{"role":"assistant","content":"测试"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`

func object(t *testing.T, s string) model.Object {
	t.Helper()
	var o model.Object
	if err := json.Unmarshal([]byte(s), &o); err != nil {
		t.Fatal(err)
	}
	return o
}
func chunk(content, reason string) string {
	finish := any(nil)
	if reason != "" {
		finish = reason
	}
	b, _ := json.Marshal(map[string]any{"id": "fixture-stream", "object": "chat.completion.chunk", "created": 1, "model": "upstream", "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": content}, "finish_reason": finish}}})
	return string(b)
}
func chatServer(t *testing.T, backend model.ChatBackend, limit int, timeout time.Duration) *Server {
	t.Helper()
	s, err := New(Options{Registry: testRegistry(t), Chats: map[string]model.ChatBackend{"fake": backend}, MaxConcurrent: limit, Timeout: timeout, Authenticate: func(token string) (Client, error) {
		if token != "fixture-token" {
			return Client{}, fmt.Errorf("invalid")
		}
		return Client{ID: "test", AllowedModels: []string{"xcli/text-default"}, AllowRemote: true}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func post(t *testing.T, s http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "http://127.0.0.1:18741/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer fixture-token")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestChatNonstreamContractAndHardGates(t *testing.T) {
	var calls atomic.Int32
	b := &fake.Backend{CompleteFunc: func(ctx context.Context, r model.Object) (model.Object, error) {
		calls.Add(1)
		if r.String("model") != "upstream" || string(r["temperature"]) != "0.3" || !bytes.Contains(r["messages"], []byte("中文")) {
			t.Error(r)
		}
		return object(t, completionBody), nil
	}}
	s := chatServer(t, b, 2, time.Second)
	w := post(t, s, chatBody)
	if w.Code != 200 || object(t, w.Body.String()).String("model") != "xcli/text-default" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, extra := range []string{`,"tools":[]`, `,"response_format":{"type":"json_schema"}`, `,"response_format":{"type":"json_object"}`} {
		w = post(t, s, strings.TrimSuffix(chatBody, "}")+extra+"}")
		if w.Code != 422 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, bad := range []string{`{}`, `null`, `{} {}`, strings.Replace(chatBody, `"synthetic"`, `{}`, 1), strings.TrimSuffix(chatBody, "}") + `,"stream":"true"}`} {
		if w = post(t, s, bad); w.Code < 400 {
			t.Fatal("accepted malformed input", bad)
		}
	}
	s.options.Authenticate = func(string) (Client, error) {
		return Client{ID: "local-only", AllowedModels: []string{"xcli/text-default"}}, nil
	}
	if w = post(t, s, chatBody); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatal("rejected requests reached backend", calls.Load())
	}
}

func TestChatConcurrencyAndTimeout(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	b := &fake.Backend{CompleteFunc: func(ctx context.Context, r model.Object) (model.Object, error) {
		entered <- struct{}{}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return object(t, completionBody), nil
		}
	}}
	s := chatServer(t, b, 1, time.Second)
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- post(t, s, chatBody) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	if w := post(t, s, chatBody); w.Code != 429 {
		t.Fatal(w.Code)
	}
	close(release)
	if w := <-finished; w.Code != 200 {
		t.Fatal(w.Code)
	}
	timed := &fake.Backend{CompleteFunc: func(ctx context.Context, r model.Object) (model.Object, error) { <-ctx.Done(); return nil, ctx.Err() }}
	s = chatServer(t, timed, 1, 20*time.Millisecond)
	if w := post(t, s, chatBody); w.Code != 504 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestRealSSEForwardingBeforeCompletionAndCancellation(t *testing.T) {
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(cancelled)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", chunk("你好", ""))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer upstream.Close()
	b, err := modelbackend.NewOpenAI(upstream.URL+"/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	server := httptest.NewServer(chatServer(t, b, 2, time.Second))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/v1/chat/completions", strings.NewReader(strings.TrimSuffix(chatBody, "}")+`,"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fixture-token")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(res.Body)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "你好") || strings.Contains(line, `"model":"upstream"`) {
		t.Fatal(line, err)
	}
	cancel()
	res.Body.Close()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream context not cancelled")
	}
}

func TestSSECompletionAndTruncation(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(fmt.Sprint(complete), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: %s\n\n", chunk("好", ""))
				if complete {
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk("", "stop"))
				}
			}))
			defer upstream.Close()
			b, _ := modelbackend.NewOpenAI(upstream.URL, "")
			defer b.Close()
			w := post(t, chatServer(t, b, 2, time.Second), strings.TrimSuffix(chatBody, "}")+`,"stream":true}`)
			body := w.Body.String()
			if w.Code != 200 || strings.Contains(body, "data: [DONE]\n\n") != complete || strings.Contains(body, `"error"`) == complete {
				t.Fatal(w.Code, body)
			}
		})
	}
}

func TestGatewayBodyLimitAndNoUnknownUsageFabrication(t *testing.T) {
	b := &fake.Backend{CompleteFunc: func(context.Context, model.Object) (model.Object, error) {
		out := object(t, completionBody)
		delete(out, "usage")
		return out, nil
	}}
	s := chatServer(t, b, 2, time.Second)
	w := post(t, s, chatBody)
	if w.Code != 200 || strings.Contains(w.Body.String(), "usage") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = post(t, s, `{"padding":"`+strings.Repeat("a", maxJSONBody)+`"}`)
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
	_, _ = io.Copy(io.Discard, w.Body)
}
