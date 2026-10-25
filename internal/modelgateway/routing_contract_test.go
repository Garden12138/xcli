package modelgateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/model"
	"github.com/Garden12138/xcli/internal/modelbackend/fake"
)

func TestVerifiedJSONAndToolCallsPassThrough(t *testing.T) {
	var captured model.Object
	b := &fake.Backend{CompleteFunc: func(_ context.Context, r model.Object) (model.Object, error) {
		captured = r
		return object(t, strings.Replace(completionBody, `"测试"`, `"{\"ok\":true}"`, 1)), nil
	}, StreamFunc: func(_ context.Context, r model.Object, send func(model.Object) error) error {
		captured = r
		for _, raw := range []string{
			`{"id":"tools","object":"chat.completion.chunk","created":1,"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"decide","arguments":"{"}}]},"finish_reason":null}]}`,
			`{"id":"tools","object":"chat.completion.chunk","created":1,"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ok\":true}"}}]},"finish_reason":"tool_calls"}]}`,
			`{"id":"tools","object":"chat.completion.chunk","created":1,"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`,
		} {
			if err := send(object(t, raw)); err != nil {
				return err
			}
		}
		return nil
	}}
	s := chatServer(t, b, 2, time.Second)
	d := s.options.Registry.List()[0]
	d.Metadata.Capabilities = append(d.Metadata.Capabilities, model.Tools, model.JSONObject, model.JSONSchema)
	d.Metadata.Verification = "probe_verified"
	d.Metadata.VerifiedAt = "2026-10-25T00:00:00Z"
	d.Metadata.Evidence = "offline-structured-fixture"
	registry, err := model.NewRegistry([]model.Descriptor{d})
	if err != nil {
		t.Fatal(err)
	}
	s.options.Registry = registry
	jsonRequest := strings.TrimSuffix(chatBody, "}") + `,"response_format":{"type":"json_schema","json_schema":{"name":"decision","strict":true,"schema":{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}}}}`
	w := post(t, s, jsonRequest)
	if w.Code != 200 || string(captured["response_format"]) != string(object(t, jsonRequest)["response_format"]) {
		t.Fatal(w.Code, w.Body.String(), captured)
	}
	toolsRequest := strings.TrimSuffix(chatBody, "}") + `,"stream":true,"tools":[{"type":"function","function":{"name":"decide","parameters":{"type":"object"}}}],"parallel_tool_calls":false,"tool_choice":{"type":"function","function":{"name":"decide"}}}`
	w = post(t, s, toolsRequest)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "data: [DONE]\n\n") {
		t.Fatal(w.Code, w.Body.String())
	}
	arguments := ""
	callID := ""
	finished := false
	usage := false
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		event := object(t, strings.TrimPrefix(line, "data: "))
		if event["usage"] != nil {
			usage = true
			continue
		}
		var choices []struct {
			Finish string `json:"finish_reason"`
			Delta  struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Index    int    `json:"index"`
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(event["choices"], &choices); err != nil {
			t.Fatal(err)
		}
		for _, choice := range choices {
			if choice.Finish == "tool_calls" {
				finished = true
			}
			for _, tool := range choice.Delta.ToolCalls {
				if tool.ID != "" {
					callID = tool.ID
				}
				if tool.Index != 0 {
					t.Fatal(tool.Index)
				}
				arguments += tool.Function.Arguments
			}
		}
	}
	if arguments != `{"ok":true}` || callID != "call_1" || !finished || !usage || string(captured["parallel_tool_calls"]) != "false" {
		t.Fatal(arguments, callID, finished, usage, captured)
	}
}

func TestProviderSwitchRequiresExplicitAliasAndNoFailureFallback(t *testing.T) {
	primary, secondary := 0, 0
	s := chatServer(t, &fake.Backend{CompleteFunc: func(context.Context, model.Object) (model.Object, error) {
		primary++
		return nil, model.Failure(429, "upstream_rate_limit", "quota exceeded")
	}}, 2, time.Second)
	d := s.options.Registry.List()[0]
	other := d
	other.ID = "xcli/secondary"
	other.BackendID = "second"
	r, err := model.NewRegistry([]model.Descriptor{d, other})
	if err != nil {
		t.Fatal(err)
	}
	s.options.Registry = r
	s.options.Chats["second"] = &fake.Backend{CompleteFunc: func(context.Context, model.Object) (model.Object, error) {
		secondary++
		return object(t, completionBody), nil
	}}
	s.options.Authenticate = func(string) (Client, error) {
		return Client{ID: "switch", AllowedModels: []string{d.ID, other.ID}, AllowRemote: true}, nil
	}
	if w := post(t, s, chatBody); w.Code != 429 || secondary != 0 || primary != 1 {
		t.Fatal(w.Code, primary, secondary)
	}
	if w := post(t, s, strings.Replace(chatBody, d.ID, other.ID, 1)); w.Code != 200 || secondary != 1 {
		t.Fatal(w.Code, secondary)
	}
}
