package driver

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
)

type codex struct{ cfg config.AgentConfig }

func (d *codex) Name() string       { return "codex" }
func (d *codex) Command() string    { return d.cfg.Command }
func (d *codex) NativeSchema() bool { return true }
func (d *codex) NewParser() Parser  { return &codexParser{parserState: parserState{name: d.Name()}} }
func (d *codex) Probe(ctx context.Context, exe string, env []string) Health {
	return probe(ctx, d, exe, env, "login", "status")
}

func (d *codex) Build(inv Invocation) ([]string, error) {
	args := []string{"exec"}
	if inv.SessionID != "" {
		args = append(args, "resume")
	}
	args = append(args, "--json", "--skip-git-repo-check")
	if inv.SessionID == "" {
		args = append(args, "-C", inv.Workdir)
	}
	sandbox := ""
	switch inv.Permission {
	case api.PermRead:
		sandbox = "read-only"
	case api.PermWrite:
		sandbox = "workspace-write"
	case api.PermFull:
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	default:
		return nil, errors.New("请将 permission 设为 read/write/full")
	}
	if sandbox != "" {
		if inv.SessionID == "" {
			args = append(args, "-s", sandbox)
		} else {
			args = append(args, "-c", `sandbox_mode="`+sandbox+`"`)
		}
	}
	if inv.Permission == api.PermWrite && inv.Network {
		args = append(args, "-c", "sandbox_workspace_write.network_access=true")
	}
	if inv.Model != "" {
		args = append(args, "-m", inv.Model)
	}
	if inv.SchemaFile != "" {
		args = append(args, "--output-schema", inv.SchemaFile)
	}
	for _, image := range inv.Images {
		args = append(args, "-i", image)
	}
	if inv.SessionID == "" {
		for _, dir := range inv.AddDirs {
			args = append(args, "--add-dir", dir)
		}
	}
	args = append(args, inv.ExtraArgs...)
	args = append(args, "--")
	if inv.SessionID != "" {
		args = append(args, inv.SessionID)
	}
	return append(args, inv.Prompt), nil
}

type codexParser struct{ parserState }

var reconnect = regexp.MustCompile(`^Reconnecting\.\.\. \d+/\d+`)

func (p *codexParser) Feed(line []byte) []Event {
	var v struct {
		Type     string
		ThreadID string `json:"thread_id"`
		Message  string
		Usage    *api.Usage
		Item     struct{ Type, Text, Command, Name, Tool string }
	}
	if json.Unmarshal(line, &v) != nil {
		return nil
	}
	switch v.Type {
	case "thread.started":
		p.result.SessionID = v.ThreadID
		return []Event{{Kind: "session", Text: v.ThreadID}}
	case "item.completed", "item.started":
		if v.Type == "item.completed" && v.Item.Type == "agent_message" {
			p.result.Text = v.Item.Text
			return []Event{{Kind: "message", Text: v.Item.Text}}
		}
		tool := v.Item.Command
		if tool == "" {
			tool = v.Item.Name
		}
		if tool == "" {
			tool = v.Item.Tool
		}
		if tool != "" {
			return []Event{{Kind: "tool", Tool: tool}}
		}
	case "turn.completed":
		p.done, p.result.Usage = true, v.Usage
		if v.Usage != nil && v.Usage.TotalTokens == 0 {
			v.Usage.TotalTokens = v.Usage.InputTokens + v.Usage.OutputTokens
		}
	case "error":
		if reconnect.MatchString(v.Message) {
			return []Event{{Kind: "notice", Text: v.Message}}
		}
		p.fail(string(line))
	case "turn.failed":
		p.done = true
		p.fail(string(line))
	}
	return nil
}

func (p *codexParser) Finish(exitCode int, stderrTail string) Result {
	return p.finish(exitCode, stderrTail)
}
