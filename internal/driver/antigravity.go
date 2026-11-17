package driver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
)

// 根据 docs/v2/driver-notes.md 的原生配置实测，定向放行 ls/pwd，写入和其他终端操作由原生权限规则处理。
const antigravityReadSettings = `{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"allow":["command(ls)","command(pwd)"],"ask":[],"deny":["write_file(*)","mcp(*)"]}}`

type antigravity struct{ cfg config.AgentConfig }

func (d *antigravity) Name() string       { return "antigravity" }
func (d *antigravity) Command() string    { return d.cfg.Command }
func (d *antigravity) NativeSchema() bool { return true }
func (d *antigravity) NewParser() Parser {
	return &antigravityParser{parserState: parserState{name: d.Name()}}
}
func (d *antigravity) Probe(ctx context.Context, exe string, env []string) Health {
	return probe(ctx, d, exe, env)
}

func (d *antigravity) Build(inv Invocation) ([]string, error) {
	args := []string{"--output-format", "stream-json", "--print-timeout", "0"}
	switch inv.Permission {
	case api.PermRead:
		args = append(args, "--sandbox", "--settings", antigravityReadSettings)
	case api.PermWrite:
		args = append(args, "--dangerously-skip-permissions", "--sandbox")
	case api.PermFull:
		args = append(args, "--dangerously-skip-permissions")
	default:
		return nil, errors.New("请将 permission 设为 read/write/full")
	}
	if inv.Model != "" {
		args = append(args, "--model", inv.Model)
	}
	if inv.SchemaFile != "" {
		args = append(args, "--json-schema", inv.SchemaFile)
	}
	for _, dir := range inv.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	if inv.SessionID != "" {
		args = append(args, "--conversation", inv.SessionID)
	}
	args = append(args, inv.ExtraArgs...)
	return append(args, "--print", inv.Prompt), nil
}

type antigravityParser struct{ parserState }

func (p *antigravityParser) deny(action string) {
	for _, existing := range p.result.Denied {
		if existing == action {
			return
		}
	}
	p.result.Denied = append(p.result.Denied, action)
}

func (p *antigravityParser) Feed(line []byte) []Event {
	var v struct {
		Event          string
		ConversationID string `json:"conversation_id"`
		StepUpdate     struct {
			ConversationID string `json:"conversation_id"`
			State          string
			ToolName       string `json:"tool_name"`
			ToolInfo       struct {
				Error struct{ Message string }
			} `json:"tool_info"`
		} `json:"step_update"`
		Result struct {
			ConversationID   string `json:"conversation_id"`
			Status, Response string
			Error            json.RawMessage
			StructuredOutput json.RawMessage   `json:"structured_output"`
			DeniedActions    []json.RawMessage `json:"denied_actions"`
			Usage            *api.Usage
		}
	}
	if json.Unmarshal(line, &v) != nil {
		return nil
	}
	events := []Event{}
	session := v.ConversationID
	switch v.Event {
	case "step_update":
		s := v.StepUpdate
		session = s.ConversationID
		if s.ToolName != "" {
			events = append(events, Event{Kind: "tool", Tool: s.ToolName})
		}
		message := strings.ToLower(s.ToolInfo.Error.Message)
		if s.State == "ERROR" && (strings.Contains(message, "permission check failed") || strings.Contains(message, "permission denied")) {
			action := s.ToolName
			if action == "run_command" {
				action = "command"
			}
			p.deny(action)
		}
	case "result":
		p.done = true
		r := v.Result
		session = r.ConversationID
		p.result.Text, p.result.Usage = r.Response, r.Usage
		if len(r.StructuredOutput) > 0 && string(r.StructuredOutput) != "null" {
			p.result.Text = string(r.StructuredOutput)
		}
		for _, raw := range r.DeniedActions {
			var action string
			if json.Unmarshal(raw, &action) != nil {
				var item struct{ Action string }
				_ = json.Unmarshal(raw, &item)
				action = item.Action
			}
			if action != "" {
				p.deny(action)
			}
		}
		if r.Status != "SUCCESS" {
			p.fail(string(r.Error))
		}
		events = append(events, Event{Kind: "message", Text: p.result.Text})
	}
	if session != "" && session != p.result.SessionID {
		p.result.SessionID = session
		events = append(events, Event{Kind: "session", Text: session})
	}
	return events
}

func (p *antigravityParser) Finish(exitCode int, stderrTail string) Result {
	if len(p.result.Denied) > 0 {
		p.result.ErrorKind = ErrPermissionDenied
	}
	return p.finish(exitCode, stderrTail)
}
