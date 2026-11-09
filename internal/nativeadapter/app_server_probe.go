package nativeadapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/codexrpc"
	"github.com/Garden12138/xcli/internal/isolatedruntime"
)

type AppServerProbe struct {
	Decision            string            `json:"decision"`
	BinarySHA256        string            `json:"binary_sha256"`
	OuterIsolation      bool              `json:"outer_isolation"`
	Initialized         bool              `json:"initialized"`
	ModelRequests       int               `json:"model_requests"`
	RealCredentialsUsed bool              `json:"real_credentials_used"`
	ProcessStarts       int               `json:"process_starts"`
	Checks              map[string]string `json:"checks"`
	ExitCode            *int              `json:"exit_code"`
	LocalStopConfirmed  bool              `json:"local_stop_confirmed"`
}

// Owner-only probe. Its transport cannot issue thread/turn/auth/config methods.
// It reads only files it just created and never uses an existing CODEX_HOME.
func ProbeAppServer(ctx context.Context, binary string, outer bool) (AppServerProbe, error) {
	out := AppServerProbe{Decision: agentpolicy.IsolatedRuntimeRequired, OuterIsolation: outer, Checks: map[string]string{}}
	canonical, e := filepath.EvalSymlinks(binary)
	if e != nil || !filepath.IsAbs(binary) {
		return out, errors.New("absolute_selected_codex_required")
	}
	binary = canonical
	bytes, e := os.ReadFile(binary)
	if e != nil {
		return out, e
	}
	out.BinarySHA256 = api.Hash(bytes)
	base, e := os.MkdirTemp("", "xcli-g6-rpc-")
	if e != nil {
		return out, e
	}
	defer os.RemoveAll(base)
	base, e = filepath.EvalSymlinks(base)
	if e != nil {
		return out, e
	}
	home, ws, auth := filepath.Join(base, "home"), filepath.Join(base, "workspace"), filepath.Join(base, "fake-auth")
	for _, dir := range []string{home, ws, auth} {
		if e = os.Mkdir(dir, 0700); e != nil {
			return out, e
		}
	}
	const marker = "XCLI_G6_SYNTHETIC_CANARY_NOT_A_LOGIN\n"
	selected := filepath.Join(ws, "material.txt")
	credential := filepath.Join(auth, "auth.json")
	for _, path := range []string{selected, credential} {
		if e = os.WriteFile(path, []byte(marker), 0600); e != nil {
			return out, e
		}
	}
	link := filepath.Join(ws, "outside-link")
	if e = os.Symlink(credential, link); e != nil {
		return out, e
	}
	spec := agent.CommandSpec{Command: binary, Args: AppServerArgs()}
	if outer {
		spec, e = isolatedruntime.Seatbelt(binary, ws, home, AppServerArgs())
		if e != nil {
			return out, e
		}
	}
	processCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, e := codexrpc.Start(processCtx, spec, ws, append(agentpolicy.Environment(nil, home), "CODEX_HOME="+auth, "XCLI_G6_FAKE_SECRET=CANARY_ONLY"), true)
	if e != nil {
		return out, e
	}
	defer c.Close()
	out.ProcessStarts = 1
	request := func(method string, params any) (json.RawMessage, bool, error) {
		if _, e := c.Request(method, params); e != nil {
			return nil, false, e
		}
		timeout := time.NewTimer(4 * time.Second)
		defer timeout.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil, false, ctx.Err()
			case <-timeout.C:
				return nil, false, errors.New("probe_reply_timeout")
			case frame, open := <-c.Frames:
				if !open {
					return nil, false, errors.New("probe_eof")
				}
				if frame.Err != nil {
					return nil, false, frame.Err
				}
				m := frame.Message
				if m.Method != "" {
					if len(m.ID) > 0 {
						if e := c.Reject(m, nil); e != nil {
							return nil, false, e
						}
					}
					continue
				}
				got, e := c.Accept(m)
				if e != nil || got != method {
					return nil, false, codexrpc.ErrProtocol
				}
				if len(m.Error) > 0 {
					return nil, false, nil
				}
				return m.Result, true, nil
			}
		}
	}
	_, ok, probeErr := request("initialize", InitParams())
	out.Initialized = ok && probeErr == nil
	if out.Initialized {
		probeErr = c.Initialized()
		read := func(name, path string) {
			raw, ok, e := request("fs/readFile", map[string]any{"path": path})
			result := "ERROR"
			if e == nil {
				result = "DENIED"
				if ok {
					var body struct {
						Data string `json:"dataBase64"`
					}
					if json.Unmarshal(raw, &body) == nil {
						data, e := base64.StdEncoding.DecodeString(body.Data)
						if e == nil && string(data) == marker {
							result = "READ_EXACT_CANARY"
						}
					}
				}
			}
			out.Checks[name] = result
		}
		read("direct_fs_selected", selected)
		read("direct_fs_fake_auth_outside", credential)
		read("direct_fs_symlink_outside", link)
		for _, defaults := range []bool{false, true} {
			name := "restricted_defaults_false"
			if defaults {
				name = "restricted_defaults_true"
			}
			raw, ok, e := request("command/exec", map[string]any{"command": []string{"/bin/cat", selected}, "cwd": ws, "sandboxPolicy": AppServerSandbox(ws, defaults), "timeoutMs": 1000})
			result := "RPC_REJECTED"
			if e == nil && ok {
				var command struct {
					ExitCode int
					Stdout   string
				}
				if json.Unmarshal(raw, &command) == nil && command.ExitCode == 0 && command.Stdout == marker {
					result = "READ_EXACT_CANARY"
				} else {
					result = "COMMAND_DENIED_OR_FAILED"
				}
			}
			out.Checks[name] = result
		}
		out.Checks["all_model_builtin_tools"] = "NOT_TESTED_NO_REAL_TURN"
		out.Checks["authenticated_model_connectivity"] = "NOT_TESTED_CREDENTIAL_FREE_OFFLINE"
	}
	c.CloseInput()
	select {
	case outcome := <-c.Done:
		out.ExitCode = &outcome.Process.ExitCode
		out.LocalStopConfirmed = outcome.Process.StopConfirmed && !outcome.Process.WaitIncomplete
	case <-time.After(250 * time.Millisecond):
		cancel()
		select {
		case outcome := <-c.Done:
			out.ExitCode = &outcome.Process.ExitCode
			out.LocalStopConfirmed = outcome.Process.StopConfirmed && !outcome.Process.WaitIncomplete
		case <-time.After(5 * time.Second):
		}
	}
	after, e := os.ReadFile(binary)
	if e != nil || api.Hash(after) != out.BinarySHA256 {
		return out, errors.New("probe_binary_changed")
	}
	if probeErr != nil {
		return out, errors.New("app_server_non_inference_probe_failed")
	}
	return out, nil
}
