package agent

import (
	"math"
	"strings"
)

// Observation is opt-in and never includes raw tool payloads or diagnostics.
// SessionID is emitted only for an explicitly present native field, before any
// legacy runtime fallback. Existing Event/RunResult semantics remain unchanged.
type Observation struct {
	Kind       string
	Category   string
	SessionID  string
	Terminal   string
	Diagnostic string
	Usage      *ObservedUsage
}
type ObservedUsage struct {
	Scope                                      string
	Input, Output, Total, CacheRead, Reasoning *int64
}

func (d *StreamDecoder) SetObserver(fn func(Observation) error) { d.observer = fn }
func (d *StreamDecoder) ObserverError() error                   { return d.observerErr }
func (d *StreamDecoder) notify(o Observation) {
	if d.observer != nil && d.observerErr == nil {
		d.observerErr = d.observer(o)
	}
}
func (d *StreamDecoder) observeEvent(event map[string]any) {
	kind, _ := event["type"].(string)
	body := event
	terminal := ""
	diagnostic := ""
	id := ""
	for _, key := range []string{"session_id", "sessionID", "thread_id", "threadId"} {
		if v, ok := event[key].(string); ok && v != "" {
			id = v
		}
	}
	if d.adapter == "antigravity" {
		kind, _ = event["event"].(string)
		if kind == "result" {
			if b, ok := event["result"].(map[string]any); ok {
				body = b
				id, _ = b["conversation_id"].(string)
				if b["status"] == "SUCCESS" {
					terminal = "success"
				} else {
					terminal = "failure"
				}
				if denied, ok := b["denied_actions"].([]any); ok && len(denied) > 0 {
					diagnostic = "approval_required"
				}
			}
		}
		if kind == "step_update" {
			if b, ok := event["step_update"].(map[string]any); ok {
				id, _ = b["conversation_id"].(string)
			}
		}
	}
	if d.adapter == "codex" {
		if kind == "turn.completed" {
			terminal = "success"
		}
		if kind == "turn.failed" {
			terminal = "failure"
		}
		if (kind == "error" || kind == "turn.failed") && codexUsageLimit(event) {
			diagnostic = "native_quota_exceeded"
		}
	}
	if d.adapter == "cursor" && kind == "result" {
		terminal = "success"
		if subtype, ok := event["subtype"].(string); ok && subtype != "" && subtype != "success" {
			terminal = "failure"
		}
		if v, _ := event["is_error"].(bool); v {
			terminal = "failure"
		}
	}
	if id != "" {
		if !safeNativeID(id) {
			d.notify(Observation{Kind: "diagnostic", Diagnostic: "protocol_error"})
		} else {
			d.notify(Observation{Kind: "native_session", Category: "session", SessionID: id})
		}
	}
	if diagnostic != "" {
		d.notify(Observation{Kind: "diagnostic", Diagnostic: diagnostic})
	}
	if terminal != "" {
		d.notify(Observation{Kind: "terminal", Terminal: terminal})
	}
	if values, ok := body["usage"].(map[string]any); ok {
		scope := "unknown"
		if d.adapter == "codex" && kind == "turn.completed" {
			scope = "turn"
		}
		u := ObservedUsage{Scope: scope, Input: nullableToken(values, "input_tokens"), Output: nullableToken(values, "output_tokens"), Total: nullableToken(values, "total_tokens"), CacheRead: nullableToken(values, "cache_read_tokens"), Reasoning: nullableToken(values, "thinking_tokens")}
		if d.adapter == "codex" {
			u.CacheRead = nullableToken(values, "cached_input_tokens")
			u.Reasoning = nullableToken(values, "reasoning_output_tokens")
		}
		d.notify(Observation{Kind: "usage", Usage: &u})
	}
	if kind != "" {
		category := "activity"
		switch kind {
		case "thread.started", "system", "init":
			category = "session"
		case "step_update", "tool_call", "tool_use":
			category = "tool"
		case "assistant", "message", "item.completed":
			category = "response"
		}
		d.notify(Observation{Kind: "activity", Category: category})
	}
}
func nullableToken(m map[string]any, k string) *int64 {
	v, ok := m[k].(float64)
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v >= 9223372036854775808.0 || math.Trunc(v) != v {
		return nil
	}
	n := int64(v)
	return &n
}

// GatewayDiagnostics is a versioned, bounded operational classifier. It scans
// stderr only and keeps findings across later floods, never response text.
type GatewayDiagnostics struct {
	Adapter string
	window  string
	code    string
}

func (d *GatewayDiagnostics) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 {
		size := min(len(data), 1024)
		d.window += strings.ToLower(string(data[:size]))
		data = data[size:]
		code := ""
		switch {
		case d.Adapter == "cursor" && (strings.Contains(d.window, "workspace is not trusted") || strings.Contains(d.window, "workspace trust") || strings.Contains(d.window, "untrusted workspace")):
			code = "workspace_untrusted"
		case strings.Contains(d.window, "not authenticated") || strings.Contains(d.window, "not logged in") || strings.Contains(d.window, "authentication required"):
			code = "native_auth_required"
		case strings.Contains(d.window, "usage limit reached") || strings.Contains(d.window, "quota exceeded"):
			code = "native_quota_exceeded"
		case strings.Contains(d.window, "additional payment required") || strings.Contains(d.window, "purchase credits to continue"):
			code = "extra_payment_required"
		case d.Adapter == "antigravity" && strings.Contains(d.window, "headless mode cannot prompt") && strings.Contains(d.window, "auto-denied"):
			code = "approval_required"
		}
		if code != "" && d.code == "" {
			d.code = code
		}
		if len(d.window) > 4096 {
			d.window = d.window[len(d.window)-4096:]
		}
	}
	return n, nil
}
func (d *GatewayDiagnostics) Code() string { return d.code }

func safeNativeID(s string) bool {
	if len(s) < 1 || len(s) > 256 {
		return false
	}
	for i, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || (i > 0 && (c == '_' || c == '-')) {
			continue
		}
		return false
	}
	return true
}
