package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const MaxEventBytes = 8 << 20
const MaxOutputBytes = 8 << 20

// Event is the versioned public stream. Raw tool payloads are never included.
type Event struct {
	Version int       `json:"version"`
	RunID   string    `json:"run_id"`
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	StepID  string    `json:"step_id,omitempty"`
	Attempt int       `json:"attempt,omitempty"`
	Message string    `json:"message,omitempty"`
	Result  any       `json:"result,omitempty"`
}

type Capability struct {
	Supported bool   `json:"supported"`
	Source    string `json:"source"`
}

func (d Definition) Capabilities() map[string]Capability {
	known := d.Config.Adapter != "generic"
	return map[string]Capability{
		"headless":          {true, "declared"},
		"structured_output": {known || d.Config.Output == "json" || d.Config.Output == "jsonl", "declared"},
		"resume":            {known, "declared"},
		"cwd":               {true, "runtime"},
		"cancel":            {true, "runtime"},
		"read_only":         {false, "unknown"},
		"continuous_input":  {false, "unknown"},
	}
}

// StreamDecoder consumes arbitrary chunks, drains overlong events, and keeps
// parsing after a damaged event so session and usage evidence are retained.
type StreamDecoder struct {
	adapter, format string
	pending         []byte
	dropping        bool
	line            int
	output          string
	terminal        bool
	result          RunResult
	usage           *usageAccumulator
	progress        func(string)
}

func NewStreamDecoder(adapter, format string, progress func(string)) *StreamDecoder {
	return &StreamDecoder{adapter: adapter, format: format, result: RunResult{Status: "success", ParseStatus: "complete"}, usage: newUsageAccumulator(), progress: progress}
}

func (d *StreamDecoder) fail(status, reason string) {
	if d.result.ParseStatus == "truncated" {
		return
	}
	d.result.ParseStatus = status
	d.result.ErrorKind = status
	d.result.Error = fmt.Sprintf("%s event %d: %s", d.adapter, d.line, reason)
	d.result.Status = "failed"
}

func (d *StreamDecoder) text() bool {
	return d.adapter == "generic" && (d.format == "" || d.format == "text")
}

func (d *StreamDecoder) setOutput(value string, appendText bool) {
	if appendText {
		available := MaxOutputBytes - len(d.output)
		if len(value) > available {
			d.output += value[:available]
			d.fail("truncated", "final output exceeds 8 MiB")
			return
		}
		d.output += value
	} else {
		if len(value) > MaxOutputBytes {
			d.output = value[:MaxOutputBytes]
			d.fail("truncated", "final output exceeds 8 MiB")
			return
		}
		d.output = value
	}
}

func (d *StreamDecoder) Write(data []byte) (int, error) {
	n := len(data)
	if d.text() {
		d.setOutput(string(data), true)
		return n, nil
	}
	// Generic JSON is a single document and can include pretty-printed newlines.
	if d.adapter == "generic" && d.format == "json" {
		if len(d.pending)+len(data) > MaxEventBytes {
			d.pending = nil
			d.dropping = true
			d.fail("truncated", "JSON document exceeds 8 MiB")
		} else if !d.dropping {
			d.pending = append(d.pending, data...)
		}
		return n, nil
	}
	for len(data) > 0 {
		index := bytes.IndexByte(data, '\n')
		fragment := data
		if index >= 0 {
			fragment = data[:index]
		}
		if !d.dropping {
			if len(d.pending)+len(fragment) > MaxEventBytes {
				d.pending = nil
				d.dropping = true
				d.fail("truncated", "event exceeds 8 MiB")
			} else {
				d.pending = append(d.pending, fragment...)
			}
		}
		if index < 0 {
			break
		}
		d.line++
		if !d.dropping {
			d.consume(d.pending)
		}
		d.pending = d.pending[:0]
		d.dropping = false
		data = data[index+1:]
	}
	return n, nil
}

func (d *StreamDecoder) consume(data []byte) {
	if len(bytes.TrimSpace(data)) == 0 {
		return
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil || event == nil {
		d.fail("parse_error", "invalid JSON object")
		return
	}
	d.usage.capture(d.adapter, event)
	captureSession(event, &d.result)
	kind, _ := event["type"].(string)
	failed := kind == "error" || kind == "turn.failed"
	if flag, _ := event["is_error"].(bool); flag {
		failed = true
	}
	// Output extraction remains adapter-aware; unknown events cannot manufacture
	// a completion or turn arbitrary diagnostic strings into an answer.
	var pieces []string
	captureOutput(event, &pieces)
	replace := false
	switch d.adapter {
	case "codex":
		d.terminal = d.terminal || kind == "turn.completed"
		replace = kind == "item.completed"
	case "claude", "cursor":
		if kind == "result" {
			d.terminal = true
			replace = true
			if subtype, _ := event["subtype"].(string); subtype != "" && subtype != "success" {
				failed = true
			}
		}
	case "gemini":
		if kind == "result" {
			d.terminal = true
			replace = true
			if status, _ := event["status"].(string); status != "success" {
				failed = true
			}
		}
	case "opencode":
		if kind == "step_finish" {
			part, _ := event["part"].(map[string]any)
			// A tool-call step is not a finished assistant turn.
			reason, _ := part["reason"].(string)
			if reason == "stop" {
				d.terminal = true
			}
			if reason == "error" {
				failed = true
			}
		}
	case "antigravity":
		pieces = nil
		name, _ := event["event"].(string)
		if name == "result" {
			d.terminal = true
			replace = true
			body, ok := event["result"].(map[string]any)
			if !ok {
				failed = true
			} else {
				d.result.SessionID, _ = body["conversation_id"].(string)
				status, _ := body["status"].(string)
				if status != "SUCCESS" {
					failed = true
				}
				if response, ok := body["response"].(string); ok {
					pieces = []string{response}
				}
				if denied, ok := body["denied_actions"].([]any); ok && len(denied) > 0 && d.result.ParseStatus == "complete" {
					// Native print mode can return SUCCESS with an earlier answer
					// after a tool is denied. Preserve the answer and parse evidence,
					// but never normalize that explicit permission failure to success.
					d.result.Status = "failed"
					d.result.ErrorKind = "process_failed"
					d.result.Error = "antigravity: native result reported denied tool permissions; allow the required operation before retrying"
				}
				d.usage.captureAntigravity(body)
			}
		}
		if name == "step_update" {
			if body, ok := event["step_update"].(map[string]any); ok {
				if id, ok := body["conversation_id"].(string); ok {
					d.result.SessionID = id
				}
			}
		}
		kind = name
	case "generic":
		d.terminal = true
		replace = d.format == "json"
	default:
		d.fail("parse_error", "unsupported adapter")
		return
	}
	if failed {
		d.fail("parse_error", "native protocol reported failure")
	}
	if len(pieces) > 0 {
		d.setOutput(strings.Join(pieces, ""), !replace)
	}
	if d.progress != nil && kind != "" {
		message := "agent activity"
		switch kind {
		case "thread.started", "system", "init":
			message = "session started"
		case "tool_call", "step_update", "tool_use":
			message = "tool activity"
		case "assistant", "message", "item.completed", "text":
			message = "response received"
		case "turn.completed", "result", "step_finish":
			message = "turn result received"
		}
		d.progress(message)
	}
}

func (d *StreamDecoder) Finish() RunResult {
	if d.text() {
		if d.result.ParseStatus == "complete" {
			d.result.ParseStatus = "not_applicable"
		}
	} else {
		if len(d.pending) > 0 && !d.dropping {
			d.line++
			d.consume(d.pending)
			d.pending = nil
		}
		if !d.terminal && d.result.ParseStatus == "complete" {
			d.fail("parse_error", "missing completion event")
		}
	}
	d.result.Output = strings.TrimSpace(d.output)
	d.result.Usage = d.usage.result()
	return d.result
}

// Normalize keeps native exit evidence separate from xcli's exit policy.
func Normalize(result RunResult, nativeExit int, canceled, timedOut bool, startErr error) (RunResult, int) {
	result.ExitCode = nativeExit
	switch {
	case startErr != nil:
		result.Status = "failed"
		result.ExitCode = -1
		result.ErrorKind = "start_failed"
		result.Error = "native process could not start"
		return result, 1
	case timedOut:
		result.Status = "timed_out"
		result.ErrorKind = "timeout"
		result.Error = "execution timed out"
		return result, 124
	case canceled:
		result.Status = "canceled"
		result.ErrorKind = "canceled"
		result.Error = "execution canceled"
		return result, 130
	case nativeExit != 0:
		result.Status = "failed"
		result.ErrorKind = "process_failed"
		result.Error = fmt.Sprintf("native process exited with code %d", nativeExit)
		if nativeExit > 0 {
			return result, nativeExit
		}
		return result, 1
	case result.ParseStatus == "parse_error" || result.ParseStatus == "truncated":
		result.Status = "failed"
		return result, 1
	case result.Status == "failed":
		// Explicit native diagnostics may contradict exit 0 and a well-formed
		// completion event. Keep that evidence instead of manufacturing success.
		return result, 1
	default:
		result.Status = "success"
		return result, 0
	}
}

func (a *usageAccumulator) captureAntigravity(event map[string]any) {
	values, ok := mapValue(event, "usage")
	if !ok {
		return
	}
	input, hasInput := tokenValue(values, "input_tokens")
	output, hasOutput := tokenValue(values, "output_tokens")
	total, hasTotal := tokenValue(values, "total_tokens")
	cache, _ := tokenValue(values, "cache_read_tokens")
	thinking, _ := tokenValue(values, "thinking_tokens")
	if !hasInput && !hasOutput && !hasTotal {
		return
	}
	a.usage = Usage{InputTokens: input, OutputTokens: output, TotalTokens: total, CacheReadTokens: cache, ReasoningTokens: thinking}
	a.seen = true
}
