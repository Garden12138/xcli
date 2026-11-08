package nativeadapter

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Garden12138/xcli/internal/agent"
	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/config"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
)

type Exec struct{}

// Specification is fixed by the registered adapter. Request data never becomes
// native options, environment entries, account/model selection or a shell.
func Specification(b BoundRequest) (agent.Definition, agent.CommandSpec, error) {
	if b.Profile.Limited != nil {
		if b.Profile.AgentID != "codex" || b.Profile.AdapterID != "codex-exec-v1" || b.NativeID != "" || b.Operation != "text_task" || b.Prompt != agentpolicy.LimitedPrompt || b.Workspace != b.Profile.Limited.Workspace {
			return agent.Definition{}, agent.CommandSpec{}, api.Failure(403, "policy_denied")
		}
		d := agent.Definition{Name: "codex", Config: config.AgentConfig{Adapter: "codex", Command: b.Profile.Binary, Output: "jsonl"}}
		args := append(agentpolicy.LimitedExecArgs(b.Workspace), "--", "xcli user task:\n"+b.Prompt)
		return d, agent.CommandSpec{Command: b.Profile.Binary, Args: args}, nil
	}
	adapter := ""
	extra := []string{}
	switch b.Profile.AdapterID {
	case "codex-exec-v1":
		adapter = "codex"
		extra = []string{"--sandbox", "read-only", "-c", "approval_policy=\"never\""}
	case "antigravity-print-v1":
		adapter = "antigravity"
		extra = []string{"--mode", "plan", "--sandbox"}
	case "cursor-headless-v1":
		adapter = "cursor"
		extra = []string{"--mode", "ask", "--sandbox", "enabled"}
	default:
		return agent.Definition{}, agent.CommandSpec{}, api.Failure(503, "adapter_unavailable")
	}
	d := agent.Definition{Name: b.Profile.AgentID, Config: config.AgentConfig{Adapter: adapter, Command: b.Profile.Binary, Output: "jsonl"}}
	prompt := "xcli user task:\n" + b.Prompt
	var spec agent.CommandSpec
	var err error
	if b.NativeID == "" {
		spec, err = d.Run(prompt, true, extra)
	} else {
		spec, err = d.Resume(b.NativeID, prompt, true, extra)
	}
	return d, spec, err
}
func (Exec) StartTurn(ctx context.Context, b BoundRequest, observe func(Observation) error) TurnResult {
	result := TurnResult{Terminal: "missing", Protocol: "not_started", LocalStopConfirmed: true, Usage: api.Usage{Source: "none", Scope: "unknown"}}
	if e := b.Profile.Check(b.Operation); e != nil {
		result.Code = e.Error()
		return result
	}
	if e := b.Profile.CheckBinary(); e != nil {
		result.Code = e.Error()
		return result
	}
	d, spec, e := Specification(b)
	if e != nil {
		result.Code = e.Error()
		return result
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	decoder := agent.NewStreamDecoder(d.Config.Adapter, "jsonl", nil)
	decoder.SetObserver(func(o agent.Observation) error {
		switch o.Kind {
		case "final_text":
			result.Text = o.Text
			if len(o.Text) > api.MaxText {
				result.Code = "output_limit"
				cancel()
			}
		case "native_session":
			if result.NativeID != "" && result.NativeID != o.SessionID {
				result.Code = "native_session_lost"
				cancel()
			}
			result.NativeID = o.SessionID
			result.SessionObserved = true
		case "terminal":
			result.Terminal = o.Terminal
		case "diagnostic":
			result.Code = o.Diagnostic
			cancel()
		case "usage":
			u := o.Usage
			result.Usage = api.Usage{Source: "native_cli", Scope: u.Scope, InputTokens: u.Input, OutputTokens: u.Output, TotalTokens: u.Total, CacheReadTokens: u.CacheRead, ReasoningTokens: u.Reasoning}
		}
		if o.Kind == "activity" && observe != nil {
			if err := observe(Observation{Kind: "activity", Category: o.Category}); err != nil {
				cancel()
				return err
			}
		}
		return nil
	})
	diagnostics := &agent.GatewayDiagnostics{Adapter: d.Config.Adapter}
	home := filepath.Join(b.Workspace, "home")
	if b.Profile.Limited != nil {
		home = filepath.Join(b.Profile.Limited.Root, "codex-limited", "home")
	}
	if e = os.MkdirAll(home, 0o700); e != nil {
		result.Code = "start_failed"
		return result
	}
	env := agentpolicy.Environment(nil, home)
	if b.Profile.Limited != nil {
		if e = b.Profile.Limited.CheckAuthorization(); e != nil {
			result.Code = e.Error()
			return result
		}
		env = append(env, "CODEX_HOME="+b.Profile.Limited.AuthHome)
	}
	process, err := xruntime.RunProcess(ctx, spec, xruntime.ProcessOptions{Dir: b.Workspace, Env: env, Stdout: decoder, Stderr: diagnostics, SeparateProcess: true, BoundedCleanup: true})
	if process.WaitIncomplete {
		return TurnResult{Code: "execution_state_unknown", Terminal: "unknown", Protocol: "unknown"}
	}
	parsed := decoder.Finish()
	result.ExitCode = &process.ExitCode
	result.Protocol = parsed.ParseStatus
	result.LocalStopConfirmed = process.StopConfirmed
	if diagnostics.Code() != "" {
		result.Code = diagnostics.Code()
	}
	if err != nil {
		result.Code = "start_failed"
	}
	if decoder.ObserverError() != nil {
		result.Code = "execution_state_unknown"
	}
	if process.IOError && result.Code == "" {
		result.Code = "native_process_failed"
	}
	// A known denial remains authoritative even if subsequent output exceeds
	// the parser limit. Truncation is a protocol fact, not permission to erase it.
	if parsed.ParseStatus == "truncated" && result.Code == "" {
		result.Code = "output_limit"
	}
	return result
}
