package driver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
)

type cursor struct{ cfg config.AgentConfig }

func (d *cursor) Name() string       { return "cursor" }
func (d *cursor) Command() string    { return d.cfg.Command }
func (d *cursor) NativeSchema() bool { return false }
func (d *cursor) NewParser() Parser {
	return &cursorParser{parserState: parserState{name: d.Name()}}
}
func (d *cursor) Probe(ctx context.Context, exe string, env []string) Health {
	return probe(ctx, d, exe, env, "status", "--format", "json")
}

func (d *cursor) Build(inv Invocation) ([]string, error) {
	args := []string{"-p", "--output-format", "stream-json", "--trust", "--workspace", inv.Workdir}
	switch inv.Permission {
	case api.PermRead:
		args = append(args, "--mode", "ask")
	case api.PermWrite:
		args = append(args, "--force", "--sandbox", "enabled")
	case api.PermFull:
		args = append(args, "--force", "--sandbox", "disabled")
	default:
		return nil, errors.New("请将 permission 设为 read/write/full")
	}
	if inv.Model != "" {
		args = append(args, "--model", inv.Model)
	}
	for _, dir := range inv.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	if inv.SessionID != "" {
		args = append(args, "--resume", inv.SessionID)
	}
	args = append(args, inv.ExtraArgs...)
	return append(args, inv.Prompt), nil
}

type cursorParser struct{ parserState }

func (p *cursorParser) Feed(line []byte) []Event {
	var v struct {
		Type, Subtype, Result string
		SessionID             string `json:"session_id"`
		IsError               bool   `json:"is_error"`
		Message               struct{ Content []struct{ Text string } }
		ToolCall              map[string]json.RawMessage `json:"tool_call"`
		Usage                 *struct{ InputTokens, OutputTokens int64 }
	}
	if json.Unmarshal(line, &v) != nil {
		return nil
	}
	events := []Event{}
	if v.SessionID != "" && v.SessionID != p.result.SessionID {
		p.result.SessionID = v.SessionID
		events = append(events, Event{Kind: "session", Text: v.SessionID})
	}
	switch v.Type {
	case "assistant":
		var text strings.Builder
		for _, content := range v.Message.Content {
			text.WriteString(content.Text)
		}
		if text.Len() > 0 {
			p.result.Text = text.String()
			events = append(events, Event{Kind: "message", Text: text.String()})
		}
	case "tool_call":
		for name := range v.ToolCall {
			if strings.HasSuffix(name, "ToolCall") {
				events = append(events, Event{Kind: "tool", Tool: name})
			}
		}
	case "result":
		p.done = true
		if p.result.Text == "" {
			p.result.Text = v.Result
		}
		if v.Usage != nil {
			p.result.Usage = &api.Usage{InputTokens: v.Usage.InputTokens, OutputTokens: v.Usage.OutputTokens, TotalTokens: v.Usage.InputTokens + v.Usage.OutputTokens}
		}
		if v.Subtype != "success" || v.IsError {
			p.fail(string(line))
		}
	}
	return events
}

func (p *cursorParser) Finish(exitCode int, stderrTail string) Result {
	return p.finish(exitCode, stderrTail)
}
