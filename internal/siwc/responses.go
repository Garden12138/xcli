package siwc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Garden12138/xcli/internal/model"
)

const MaxRequestBytes = 4 << 20
const MaxStreamBytes = 32 << 20

// ResponsesProvider is independent of ChatBackend and ImageBackend. There is
// deliberately no production implementation or registration in G8.
type ResponsesProvider interface {
	StreamResponses(context.Context, model.Object, func(Event) error) error
}

// ValidateRequest is deliberately narrower than the upstream API: no tools,
// external URLs, files, opaque reasoning items, or optional generation controls.
func ValidateRequest(req model.Object) error {
	bad := func() error {
		return model.Failure(422, "siwc_contract_rejected", "SIWC request is outside the G8 text/Vision contract")
	}
	encoded, err := json.Marshal(req)
	if err != nil || len(encoded) > MaxRequestBytes {
		return bad()
	}
	for k := range req {
		if !slices.Contains([]string{"model", "input", "instructions", "store", "stream"}, k) {
			return bad()
		}
	}
	if req.String("model") == "" || string(bytes.TrimSpace(req["store"])) != "false" || string(bytes.TrimSpace(req["stream"])) != "true" {
		return bad()
	}
	if raw, ok := req["instructions"]; ok {
		var s string
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &s) != nil {
			return bad()
		}
	}
	var input []model.Object
	if json.Unmarshal(req["input"], &input) != nil || len(input) == 0 || len(input) > 4096 {
		return bad()
	}
	for _, m := range input {
		for k := range m {
			if !slices.Contains([]string{"type", "role", "content"}, k) {
				return bad()
			}
		}
		if raw, ok := m["type"]; ok && (string(raw) == "null" || m.String("type") != "message") {
			return bad()
		}
		role := m.String("role")
		if !slices.Contains([]string{"developer", "user", "assistant"}, role) {
			return bad()
		}
		var text string
		if string(bytes.TrimSpace(m["content"])) != "null" && json.Unmarshal(m["content"], &text) == nil {
			continue
		}
		var parts []model.Object
		if json.Unmarshal(m["content"], &parts) != nil || len(parts) == 0 {
			return bad()
		}
		for _, p := range parts {
			switch p.String("type") {
			case "input_text", "output_text":
				for k := range p {
					if k != "type" && k != "text" {
						return bad()
					}
				}
				if (p.String("type") == "output_text" && role != "assistant") || string(bytes.TrimSpace(p["text"])) == "null" || json.Unmarshal(p["text"], &text) != nil {
					return bad()
				}
			case "input_image":
				for k := range p {
					if k != "type" && k != "image_url" && k != "detail" {
						return bad()
					}
				}
				if role != "user" {
					return bad()
				}
				if raw, ok := p["detail"]; ok && (string(raw) == "null" || !slices.Contains([]string{"auto", "low", "high"}, p.String("detail"))) {
					return bad()
				}
				if _, err := model.DecodeDataImage(p.String("image_url")); err != nil {
					return bad()
				}
			default:
				return bad()
			}
		}
	}
	return nil
}

// Event contains only the intended model text and a terminal marker. Raw
// upstream objects, headers, account identifiers and errors never cross it.
type Event struct {
	Type  string `json:"type"`
	Delta string `json:"delta,omitempty"`
	Text  string `json:"text,omitempty"`
}

var errorStatuses = map[string]int{
	"subscription_sharing_user_not_eligible":      403,
	"subscription_sharing_usage_limit_exceeded":   429,
	"subscription_sharing_usage_unavailable":      503,
	"subscription_sharing_unsupported_capability": 400,
	"subscription_sharing_route_not_supported":    403,
	"subscription_sharing_invalid_user":           401,
	"chatpass_v2_scope_not_authorized":            403,
	"chatpass_v2_invalid_authorization_context":   403,
	"subscription_sharing_user_unavailable":       503,
}

// UpstreamError preserves recognized stable codes; arbitrary diagnostics/body
// text are discarded. Request IDs belong in private bounded metadata, not logs
// of headers or token-bearing bodies. No error triggers billing fallback/retry.
func UpstreamError(status int, data []byte) error {
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if len(data) <= 64<<10 && json.Unmarshal(data, &out) == nil {
		if expected, ok := errorStatuses[out.Error.Code]; ok {
			return model.Failure(expected, out.Error.Code, "ChatGPT plan request was rejected")
		}
	}
	code := "siwc_upstream_error"
	switch status {
	case 401:
		code = "siwc_authentication_rejected"
	case 403:
		code = "siwc_policy_rejected"
	case 429:
		code = "siwc_rate_limit"
	case 503:
		code = "siwc_route_unavailable"
	default:
		if status < 400 || status > 599 {
			status = 502
		}
	}
	return model.Failure(status, code, "ChatGPT plan request was rejected")
}

// ConsumeStream requires a context-aware reader (the HTTP request context must
// close its body on cancellation). A terminal response.completed is mandatory.
// Reads SSE records across arbitrary network fragments, CRLF and multiline data.
func ConsumeStream(ctx context.Context, reader io.Reader, send func(Event) error) error {
	fail := func() error {
		return model.Failure(502, "siwc_invalid_stream", "Responses stream is invalid or incomplete")
	}
	scanner := bufio.NewScanner(io.LimitReader(reader, MaxStreamBytes+1))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var data []string
	total, last := 0, int64(-1)
	responseID := ""
	completed := false
	eventName := ""
	process := func() error {
		if len(data) == 0 {
			eventName = ""
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var ev model.Object
		if json.Unmarshal([]byte(strings.Join(data, "\n")), &ev) != nil || ev == nil {
			return fail()
		}
		data = nil
		kind := ev.String("type")
		if eventName != "" && eventName != kind {
			return fail()
		}
		eventName = ""
		var seq int64
		if string(ev["sequence_number"]) == "null" || json.Unmarshal(ev["sequence_number"], &seq) != nil || seq <= last {
			return fail()
		}
		last = seq
		var response model.Object
		switch kind {
		case "response.created":
			if responseID != "" || json.Unmarshal(ev["response"], &response) != nil || response.String("id") == "" || response.String("status") != "in_progress" {
				return fail()
			}
			responseID = response.String("id")
		case "response.output_text.delta":
			var delta string
			if responseID == "" || string(ev["delta"]) == "null" || json.Unmarshal(ev["delta"], &delta) != nil {
				return fail()
			}
			if err := send(Event{Type: kind, Delta: delta}); err != nil {
				return err
			}
		case "response.completed":
			if responseID == "" || json.Unmarshal(ev["response"], &response) != nil || response.String("id") != responseID || response.String("status") != "completed" {
				return fail()
			}
			text, err := completedText(response)
			if err != nil {
				return err
			}
			completed = true
			return send(Event{Type: kind, Text: text})
		case "response.failed":
			if json.Unmarshal(ev["response"], &response) != nil {
				return fail()
			}
			return UpstreamError(502, mustJSON(map[string]json.RawMessage{"error": response["error"]}))
		case "response.incomplete":
			return model.Failure(502, "siwc_incomplete", "Responses request was incomplete")
		case "error":
			return UpstreamError(502, mustJSON(map[string]json.RawMessage{"error": mustJSON(ev)}))
		default:
			if responseID == "" || !slices.Contains([]string{"response.in_progress", "response.output_item.added", "response.output_item.done", "response.content_part.added", "response.content_part.done", "response.output_text.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done"}, kind) {
				return fail()
			}
		}
		return nil
	}
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Text()
		total += len(line) + 1
		if total > MaxStreamBytes {
			return fail()
		}
		if line == "" {
			if err := process(); err != nil {
				return err
			}
			if completed {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// A missing blank record delimiter is also an interrupted stream.
	return fail()
}

func mustJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func completedText(response model.Object) (string, error) {
	var output []model.Object
	if json.Unmarshal(response["output"], &output) != nil || len(output) == 0 {
		return "", model.Failure(502, "siwc_invalid_stream", "missing completed text output")
	}
	var text strings.Builder
	for _, item := range output {
		// Reasoning is never delivered or executed. Encrypted history requires a
		// separate future contract; G8 stores only bounded explicit messages.
		if item.String("type") == "reasoning" {
			continue
		}
		if item.String("type") != "message" || item.String("role") != "assistant" || item.String("status") != "completed" {
			return "", model.Failure(502, "siwc_unexpected_output", "non-text output is disabled")
		}
		var content []model.Object
		if json.Unmarshal(item["content"], &content) != nil || len(content) == 0 {
			return "", model.Failure(502, "siwc_invalid_stream", "missing completed content")
		}
		for _, p := range content {
			var value string
			if p.String("type") != "output_text" || string(p["text"]) == "null" || json.Unmarshal(p["text"], &value) != nil {
				return "", model.Failure(502, "siwc_unexpected_output", "non-text output is disabled")
			}
			text.WriteString(value)
		}
	}
	return text.String(), nil
}

type CatalogEntry struct {
	Slug              string `json:"slug"`
	DisplayName       string `json:"display_name"`
	InferenceVerified bool   `json:"inference_verified"`
}

func ParseCatalog(data []byte) ([]CatalogEntry, error) {
	var r struct {
		Models []struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Visibility  string `json:"visibility"`
		} `json:"models"`
	}
	if len(data) > 1<<20 || json.Unmarshal(data, &r) != nil || r.Models == nil {
		return nil, fmt.Errorf("siwc_catalog_invalid")
	}
	out := []CatalogEntry{}
	seen := map[string]bool{}
	for _, m := range r.Models {
		if m.Visibility == "list" {
			if m.Slug == "" || m.DisplayName == "" || seen[m.Slug] {
				return nil, fmt.Errorf("siwc_catalog_invalid")
			}
			seen[m.Slug] = true
			out = append(out, CatalogEntry{Slug: m.Slug, DisplayName: m.DisplayName})
		}
	}
	return out, nil
}
