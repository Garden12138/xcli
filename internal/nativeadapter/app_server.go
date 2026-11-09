package nativeadapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Garden12138/xcli/internal/agent"
	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/codexrpc"
)

type AppServer struct{}

// These settings are reasserted on every resumed thread and every turn. They
// remain candidate command policies, never an attestation for all builtins.
func AppServerSandbox(workspace string, platformDefaults bool) map[string]any {
	return map[string]any{"type": "readOnly", "access": map[string]any{"type": "restricted", "readableRoots": []string{workspace}, "includePlatformDefaults": platformDefaults}}
}
func AppServerArgs() []string {
	return []string{"app-server", "--listen", "stdio://", "-c", `mcp_servers={}`, "-c", `web_search="disabled"`, "-c", `features.apps=false`, "-c", `features.plugins=false`, "-c", `features.hooks=false`, "-c", `features.multi_agent=false`, "-c", `shell_environment_policy.inherit="none"`, "-c", `shell_environment_policy.ignore_default_excludes=false`}
}
func InitParams() map[string]any {
	return map[string]any{"clientInfo": map[string]any{"name": "xcli_native_agent_gateway", "title": "xcli Native Agent Gateway", "version": "1"}, "capabilities": map[string]any{"experimentalApi": false}}
}
func nativeCode(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return "protocol_error"
	}
	var find func(any) string
	find = func(x any) string {
		switch z := x.(type) {
		case string:
			switch z {
			case "unauthorized":
				return "native_auth_required"
			case "usageLimitExceeded":
				return "native_quota_exceeded"
			case "sandboxError":
				return "policy_unenforceable"
			}
		case map[string]any:
			for _, key := range []string{"codexErrorInfo", "error", "data"} {
				if value, ok := z[key]; ok {
					if code := find(value); code != "" {
						return code
					}
				}
			}
		}
		return ""
	}
	if code := find(v); code != "" {
		return code
	}
	return "native_process_failed"
}
func (AppServer) StartTurn(ctx context.Context, b BoundRequest, observe func(Observation) error) TurnResult {
	r := TurnResult{Terminal: "missing", Protocol: "not_started", LocalStopConfirmed: true, Usage: api.Usage{Source: "none", Scope: "unknown"}}
	if e := b.Profile.Check(b.Operation); e != nil {
		r.Code = e.Error()
		return r
	}
	if b.Profile.AdapterID != agentpolicy.AppServerAdapter || b.Profile.AppServer == nil || b.Profile.AgentID != "codex" || b.Operation != "text_task" || !utf8.ValidString(b.Prompt) || len(b.Prompt) > 256<<10 {
		r.Code = "policy_denied"
		return r
	}
	if e := b.Profile.CheckBinary(); e != nil {
		r.Code = e.Error()
		return r
	}
	// A task owns exactly one process; no reconnect, fallback or implicit retry.
	home := filepath.Join(b.Profile.AppServer.Root, "app-server-runtime", b.SessionID)
	if e := os.MkdirAll(home, 0700); e != nil {
		r.Code = "start_failed"
		return r
	}
	env := append(agentpolicy.Environment(nil, home), "CODEX_HOME="+home)
	processCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	c, e := codexrpc.Start(processCtx, agent.CommandSpec{Command: b.Profile.Binary, Args: AppServerArgs()}, b.Workspace, env, false)
	if e != nil {
		r.Code = "start_failed"
		return r
	}
	defer c.Close()
	r.LocalStopConfirmed = false
	r.Protocol = "parse_error"
	fail := func(code string) {
		if r.Code == "" {
			r.Code = code
		}
		c.CloseInput()
		cancel()
	}
	if _, e = c.Request("initialize", InitParams()); e != nil {
		fail("start_failed")
	}
	frames, done := c.Frames, c.Done
	var outcome *codexrpc.Outcome
	var turnID string
	threadReady, turnReady, terminal := false, false, false
	messages := map[string]string{}
	messageOrder := []string{}
	preTurn := []codexrpc.Frame{}
	finalIDs := []string{}
	emit := func(category string) {
		if observe != nil && observe(Observation{Kind: "activity", Category: category}) != nil {
			fail("execution_state_unknown")
		}
	}
	scope := func(params map[string]json.RawMessage) bool {
		var thread, turn string
		if json.Unmarshal(params["threadId"], &thread) != nil || thread != r.NativeID {
			return false
		}
		if json.Unmarshal(params["turnId"], &turn) != nil || turn != turnID {
			return false
		}
		return turnReady
	}
	var stopTimer *time.Timer
	var stop <-chan time.Time
	scheduleStop := func(delay time.Duration) {
		if stopTimer == nil {
			stopTimer = time.NewTimer(delay)
			stop = stopTimer.C
		}
	}
	defer func() {
		if stopTimer != nil {
			stopTimer.Stop()
		}
	}()
	ctxDone := ctx.Done()
	for frames != nil || done != nil || (turnReady && len(preTurn) > 0) {
		readFrames := frames
		replay := turnReady && len(preTurn) > 0
		if replay {
			buffered := make(chan codexrpc.Frame, 1)
			buffered <- preTurn[0]
			readFrames = buffered
		}
		select {
		case <-ctxDone:
			ctxDone = nil
			if turnReady && !terminal {
				if _, e = c.Request("turn/interrupt", map[string]any{"threadId": r.NativeID, "turnId": turnID}); e != nil {
					cancel()
				}
				scheduleStop(100 * time.Millisecond)
			} else {
				c.CloseInput()
				cancel()
			}
		case <-stop:
			stop = nil
			cancel()
			c.CloseInput()
		case o := <-done:
			outcome = &o
			done = nil
			if !o.Process.StopConfirmed {
				c.Close()
				frames = nil
			}
		case frame, open := <-readFrames:
			if replay {
				preTurn = preTurn[1:]
			}
			if !open {
				frames = nil
				continue
			}
			if frame.Err != nil {
				if frame.Err.Error() == "output_limit" {
					r.Protocol = "truncated"
				}
				fail(frame.Err.Error())
				continue
			}
			m := frame.Message
			if threadReady && !turnReady && (strings.HasPrefix(m.Method, "item/") || m.Method == "turn/completed" || m.Method == "error") {
				if len(preTurn) >= 32 {
					fail("output_limit")
				} else {
					preTurn = append(preTurn, frame)
				}
				continue
			}
			if m.Method == "" {
				method, e := c.Accept(m)
				if e != nil {
					fail("protocol_error")
					continue
				}
				if len(m.Error) > 0 {
					fail(nativeCode(m.Error))
					continue
				}
				if r.Code != "" && method != "turn/interrupt" {
					continue
				}
				var object map[string]json.RawMessage
				if json.Unmarshal(m.Result, &object) != nil || object == nil {
					fail("protocol_error")
					continue
				}
				switch method {
				case "initialize":
					if ctx.Err() != nil {
						c.CloseInput()
						cancel()
						continue
					}
					if c.Initialized() != nil {
						fail("protocol_error")
						continue
					}
					params := map[string]any{"cwd": b.Workspace, "approvalPolicy": "never", "sandbox": "read-only"}
					method = "thread/start"
					if b.NativeID != "" {
						method = "thread/resume"
						params["threadId"] = b.NativeID
					}
					if _, e = c.Request(method, params); e != nil {
						fail("protocol_error")
					}
				case "thread/start", "thread/resume":
					var thread struct {
						ID string `json:"id"`
					}
					if json.Unmarshal(object["thread"], &thread) != nil || !safeNativeID(thread.ID) || threadReady || (b.NativeID != "" && b.NativeID != thread.ID) {
						fail("native_session_lost")
						continue
					}
					r.NativeID = thread.ID
					r.SessionObserved = true
					threadReady = true
					emit("session")
					if ctx.Err() != nil {
						cancel()
						c.CloseInput()
						continue
					}
					_, e = c.Request("turn/start", map[string]any{"threadId": thread.ID, "input": []any{map[string]any{"type": "text", "text": b.Prompt}}, "cwd": b.Workspace, "approvalPolicy": "never", "sandboxPolicy": AppServerSandbox(b.Workspace, true)})
					if e != nil {
						fail("protocol_error")
					}
				case "turn/start":
					var turn struct{ ID, Status string }
					if json.Unmarshal(object["turn"], &turn) != nil || !safeNativeID(turn.ID) || turnReady || !threadReady || turn.Status != "inProgress" {
						fail("protocol_error")
						continue
					}
					turnID = turn.ID
					turnReady = true
				case "turn/interrupt": // An ACK is not upstream completion.
				default:
					fail("protocol_error")
				}
				continue
			}
			var params map[string]json.RawMessage
			if json.Unmarshal(m.Params, &params) != nil {
				fail("protocol_error")
				continue
			}
			if len(m.ID) > 0 {
				var denial any
				known := false
				switch m.Method {
				case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
					denial = map[string]any{"decision": "decline"}
					known = true
				case "item/permissions/requestApproval":
					denial = map[string]any{"permissions": map[string]any{}, "scope": "turn"}
					known = true
				}
				if known && !scope(params) {
					_ = c.Reject(m, nil)
					fail("protocol_error")
					continue
				}
				if c.Reject(m, denial) != nil {
					fail("protocol_error")
					continue
				}
				if r.Code == "" {
					r.Code = "approval_required"
					if m.Method == "account/chatgptAuthTokens/refresh" {
						r.Code = "native_auth_required"
					}
				}
				c.CloseInput()
				scheduleStop(100 * time.Millisecond)
				continue
			}
			switch m.Method {
			case "item/agentMessage/delta", "item/completed", "error":
				if !scope(params) || terminal {
					fail("protocol_error")
					continue
				}
				if m.Method == "error" {
					fail(nativeCode(m.Params))
					continue
				}
				if m.Method == "item/agentMessage/delta" {
					var id, delta string
					if json.Unmarshal(params["itemId"], &id) != nil || !safeNativeID(id) || json.Unmarshal(params["delta"], &delta) != nil {
						fail("protocol_error")
						continue
					}
					if len(messages[id])+len(delta) > api.MaxText {
						fail("output_limit")
						continue
					}
					if _, exists := messages[id]; !exists {
						messageOrder = append(messageOrder, id)
					}
					messages[id] += delta
					emit("response")
				} else {
					var item struct {
						ID, Type, Text string
						Phase          *string
					}
					if json.Unmarshal(params["item"], &item) != nil {
						fail("protocol_error")
						continue
					}
					if item.Type == "agentMessage" {
						if !safeNativeID(item.ID) || len(item.Text) > api.MaxText {
							fail("output_limit")
							continue
						}
						if _, exists := messages[item.ID]; !exists {
							messageOrder = append(messageOrder, item.ID)
						}
						messages[item.ID] = item.Text
						if item.Phase == nil || *item.Phase == "final_answer" {
							finalIDs = append(finalIDs, item.ID)
						}
					} else {
						emit("tool")
					}
				}
			case "turn/completed":
				var thread string
				var turn struct {
					ID, Status string
					Error      json.RawMessage
				}
				if json.Unmarshal(params["threadId"], &thread) != nil || thread != r.NativeID || json.Unmarshal(params["turn"], &turn) != nil || !turnReady || turn.ID != turnID || terminal {
					fail("protocol_error")
					continue
				}
				terminal = true
				switch turn.Status {
				case "completed":
					r.Terminal = "success"
					r.Protocol = "complete"
				case "interrupted":
					r.Terminal = "interrupted"
					r.Protocol = "complete"
					// Native turn completion is observable; a provider cloud job
					// cancellation confirmation is not present in this protocol.
				case "failed":
					r.Terminal = "failure"
					r.Protocol = "complete"
					fail(nativeCode(turn.Error))
				default:
					fail("protocol_error")
				}
				c.CloseInput()
				scheduleStop(200 * time.Millisecond)
			case "thread/started", "turn/started", "item/started", "serverRequest/resolved", "thread/tokenUsage/updated":
				// Progress is never a terminal or usage estimate. Unknown notifications
				// likewise cannot modify ownership, text or completion.
			case "model/rerouted":
				fail("policy_unenforceable")
			}
		}
	}
	if outcome != nil {
		r.ExitCode = &outcome.Process.ExitCode
		r.LocalStopConfirmed = outcome.Process.StopConfirmed && !outcome.Process.WaitIncomplete
		if outcome.Err != nil && r.Code == "" {
			r.Code = "start_failed"
		}
		if outcome.Process.IOError && r.Code == "" {
			r.Code = "native_process_failed"
		}
	}
	if !terminal && r.Code == "" {
		r.Code = "protocol_error"
		if outcome != nil && outcome.Process.ExitCode != 0 {
			r.Code = "native_process_failed"
		}
	}
	seen := map[string]bool{}
	if len(finalIDs) == 0 && r.Code != "" {
		finalIDs = messageOrder
	}
	var text strings.Builder
	for _, id := range finalIDs {
		if !seen[id] {
			seen[id] = true
			text.WriteString(messages[id])
		}
	}
	r.Text = text.String()
	if len(r.Text) > api.MaxText {
		r.Text = ""
		r.Code = "output_limit"
	}
	return r
}
func safeNativeID(id string) bool {
	return len(id) > 0 && len(id) <= 256 && !strings.ContainsAny(id, "/\\\r\n\x00")
}
