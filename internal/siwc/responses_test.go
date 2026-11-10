package siwc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/model"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestStrictResponsesRequestContract(t *testing.T) {
	if err := ValidateRequest(validRequest()); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"background", "conversation", "max_output_tokens", "max_tool_calls", "metadata", "moderation", "multi_agent", "prompt", "prompt_cache_retention", "safety_identifier", "temperature", "top_logprobs", "top_p", "truncation", "user", "previous_response_id", "tools", "tool_choice", "additional_tools", "parallel_tool_calls", "service_tier", "unknown", "api_key", "access_token"} {
		t.Run(field, func(t *testing.T) {
			r := validRequest()
			r[field] = mustJSON(nil)
			if ValidateRequest(r) == nil {
				t.Fatal("unsupported field forwarded")
			}
		})
	}
	for _, tool := range []string{"image_generation", "file_search", "code_interpreter", "computer", "mcp", "tool_search", "programmatic_tool_calling", "function", "custom", "web_search"} {
		r := validRequest()
		r["tools"] = mustJSON([]any{map[string]any{"type": tool}})
		if ValidateRequest(r) == nil {
			t.Fatal("tool enabled", tool)
		}
	}
	for _, field := range []string{"store", "stream"} {
		for _, value := range []any{nil, "true", 0, field == "store"} {
			r := validRequest()
			r[field] = mustJSON(value)
			if ValidateRequest(r) == nil {
				t.Fatal("invalid fixed flag", field)
			}
		}
	}
	for _, input := range []any{"not array", nil, []any{}, []any{map[string]any{"type": "message", "role": "system", "content": "unsafe"}}, []any{map[string]any{"role": "tool", "content": "tool result"}}, []any{map[string]any{"role": "user", "content": nil}}, []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_file", "file_id": "file_test"}}}}} {
		r := validRequest()
		r["input"] = mustJSON(input)
		if ValidateRequest(r) == nil {
			t.Fatal("invalid input accepted")
		}
	}
	// A complete tiny PNG passes actual image decoding; URL and MIME spoofing fail.
	image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG(t))
	for _, tc := range []struct {
		url string
		ok  bool
	}{{image, true}, {"https://example.invalid/image.png", false}, {"file:///etc/passwd", false}, {"data:image/png;base64,AAAA", false}, {strings.Replace(image, "image/png", "image/jpeg", 1), false}} {
		r := validRequest()
		r["input"] = mustJSON([]any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "describe"}, map[string]any{"type": "input_image", "image_url": tc.url, "detail": "auto"}}}})
		if (ValidateRequest(r) == nil) != tc.ok {
			t.Fatal("Vision validation mismatch")
		}
	}
}

func TestStreamCompletionAndSanitizedErrors(t *testing.T) {
	for _, body := range []string{streamBody("你好"), strings.ReplaceAll(streamBody("你好"), "\n", "\r\n"), strings.ReplaceAll(streamBody("你好"), "data: {", ": heartbeat\nevent: \ndata: {"), strings.Replace(streamBody("你好"), `"sequence_number":0,`, "\"sequence_number\":0,\ndata: ", 1)} {
		var events []Event
		err := ConsumeStream(context.Background(), strings.NewReader(body), func(e Event) error { events = append(events, e); return nil })
		if err != nil || len(events) != 2 || events[1].Text != "你好" {
			t.Fatal("valid stream", err)
		}
	}
	for name, body := range map[string]string{
		"empty": "", "done-only": "data: [DONE]\n\n", "truncated": strings.Split(streamBody("ok"), "data: {\"response\":{\"id\":\"resp_synthetic\",\"output\"")[0],
		"no-terminal":       strings.Join(strings.Split(streamBody("ok"), "\n\n")[:2], "\n\n") + "\n\n",
		"missing-delimiter": strings.TrimSuffix(streamBody("ok"), "\n\n"),
		"sequence-replay":   strings.Replace(streamBody("ok"), `"sequence_number":2`, `"sequence_number":1`, 1),
		"wrong-status":      strings.Replace(streamBody("ok"), `"status":"completed"`, `"status":"failed"`, -1),
		"wrong-id":          strings.Replace(streamBody("ok"), `"id":"resp_synthetic"`, `"id":"different"`, 1),
		"untrusted-output":  strings.Replace(streamBody("ok"), `"type":"message"`, `"type":"function_call"`, 1),
		"event-name":        strings.Replace(streamBody("ok"), "data:", "event: wrong\ndata:", 1),
		"incomplete":        "data: {\"type\":\"response.incomplete\",\"sequence_number\":0}\n\n",
		"huge-record":       "data: " + strings.Repeat("x", 1<<20) + "\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := ConsumeStream(context.Background(), strings.NewReader(body), func(Event) error { return nil }); err == nil {
				t.Fatal("invalid stream accepted")
			}
		})
	}
	for code, status := range errorStatuses {
		body := `data: {"type":"response.failed","sequence_number":0,"response":{"error":{"code":"` + code + `","message":"synthetic-access-SECRET","param":"synthetic-refresh-SECRET"}}}` + "\n\n"
		err := ConsumeStream(context.Background(), strings.NewReader(body), func(Event) error { t.Fatal("error forwarded as success"); return nil })
		var e *model.Error
		if !errors.As(err, &e) || e.Code != code || e.Status != status || strings.Contains(fmt.Sprintf("%+v", e), "SECRET") {
			t.Fatal("lost classification or secret leak", code)
		}
	}
	for _, status := range []int{302, 401, 403, 429, 503, 500} {
		err := UpstreamError(status, []byte(`{"detail":"synthetic-access-SECRET"}`))
		if strings.Contains(err.Error(), "SECRET") {
			t.Fatal("raw error leaked")
		}
	}
	stop := errors.New("client_disconnected")
	if err := ConsumeStream(context.Background(), strings.NewReader(streamBody("ok")), func(Event) error { return stop }); err != stop {
		t.Fatal("consumer cancellation lost")
	}
}

func TestHistoryIsolationExpiryAndCapacity(t *testing.T) {
	h := NewHistory()
	owner := HistoryOwner{"worklog", "registration-A", "same-id"}
	var input []model.Object
	_ = json.Unmarshal(validRequest()["input"], &input)
	terminal := Event{Type: "response.completed", Text: "assistant reply"}
	if err := h.Commit(owner, input, terminal, testNow); err != nil {
		t.Fatal(err)
	}
	for _, other := range []HistoryOwner{{"gardenflow", "registration-A", "same-id"}, {"worklog", "registration-B", "same-id"}, {"worklog", "registration-A", "other"}} {
		if len(h.Load(other, testNow)) != 0 {
			t.Fatal("cross-owner history leak")
		}
	}
	input[0]["content"] = mustJSON("mutated")
	loaded := h.Load(owner, testNow)
	if len(loaded) != 2 || loaded[0].String("content") == "mutated" {
		t.Fatal("history aliased")
	}
	loaded[0]["content"] = mustJSON("mutated")
	if h.Load(owner, testNow)[0].String("content") == "mutated" {
		t.Fatal("history load aliased")
	}
	if err := h.Commit(owner, input, Event{Type: "response.incomplete"}, testNow); err == nil {
		t.Fatal("failed history committed")
	}
	for i := 1; i < 32; i++ {
		if err := h.Commit(HistoryOwner{"worklog", "registration-A", fmt.Sprint(i)}, input, terminal, testNow); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Commit(HistoryOwner{"worklog", "registration-A", "overflow"}, input, terminal, testNow); err == nil {
		t.Fatal("unbounded history")
	}
	if len(h.Load(owner, testNow.Add(30*time.Minute))) != 0 {
		t.Fatal("TTL not enforced")
	}
	if _, err := ParseCatalog([]byte(`{"data":[{"id":"api-key-model"}]}`)); err == nil {
		t.Fatal("wrong catalog contract")
	}
}

func TestProductionPackageHasNoIOOrProcessDependencies(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "os" || path == "os/exec" || path == "syscall" || path == "net/http" || path == "net" || strings.Contains(path, "nativeadapter") || strings.Contains(path, "/runtime") {
				t.Fatal("production SIWC acquired I/O or execution capability", name, path)
			}
		}
	}
}
