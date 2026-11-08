package agentpolicy

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	api "github.com/Garden12138/xcli/internal/agentapi"
	xr "github.com/Garden12138/xcli/internal/runtime"
)

type LimitedProbe struct {
	RegistrationSHA256 string          `json:"registration_sha256"`
	PolicySHA256       string          `json:"policy_sha256"`
	At                 time.Time       `json:"at"`
	NativeProcesses    int             `json:"native_processes"`
	ModelRequests      int             `json:"model_requests"`
	Login              string          `json:"login"`
	Checks             map[string]bool `json:"checks"`
	ExitCodes          map[string]int  `json:"exit_codes"`
	// Kept separate: a shell-helper test cannot prove builtin tool isolation.
	BuiltinTools string `json:"builtin_tools"`
}

func ProbeLimited(ctx context.Context, root string) (LimitedProbe, error) {
	out := LimitedProbe{At: time.Now().UTC(), Checks: map[string]bool{}, ExitCodes: map[string]int{}, BuiltinTools: "NOT_VERIFIED"}
	r, e := ReadLimited(root)
	if e != nil {
		return out, e
	}
	out.RegistrationSHA256 = api.Digest(r)
	out.PolicySHA256 = api.Digest(LimitedExecArgs(r.Workspace))
	p := Profile{Binary: r.Binary, BinarySHA256: r.BinarySHA256}
	if e = p.CheckBinary(); e != nil {
		return out, e
	}
	if CheckLimitedWorkspace(r) != nil {
		return out, api.Failure(503, "policy_unenforceable")
	}
	temp, e := os.MkdirTemp(filepath.Join(root, "codex-limited"), "probe-")
	if e != nil {
		return out, e
	}
	defer os.RemoveAll(temp)
	run := func(name string, args, env []string) (xr.ProcessResult, bool) {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		out.NativeProcesses++
		res, err := xr.RunProcess(c, agent.CommandSpec{Command: r.Binary, Args: args}, xr.ProcessOptions{Dir: r.Workspace, Env: env, CaptureStdout: true, SeparateProcess: true, BoundedCleanup: true})
		out.ExitCodes[name] = res.ExitCode
		return res, err == nil && res.StopConfirmed && !res.TimedOut && !res.Canceled && !res.CaptureTruncated && !res.IOError && len(res.Stdout) <= 64<<10
	}
	auth, ok := run("login_status", []string{"login", "status"}, append(Environment(nil, temp), "CODEX_HOME="+r.AuthHome))
	// Official status text only; never serialize stdout/stderr/account identifiers.
	status := string(auth.Stdout) + string(auth.StderrTail)
	out.Login = "UNKNOWN"
	if ok && auth.ExitCode == 0 && strings.Contains(status, "Logged in using ChatGPT") {
		out.Login = "AUTHENTICATED_CHATGPT"
	}
	help, ok := run("exec_help", []string{"exec", "--help"}, Environment(nil, temp))
	out.Checks["fixed_command_format"] = ok && help.ExitCode == 0 && strings.Contains(string(help.Stdout), "--ignore-user-config") && strings.Contains(string(help.Stdout), "--ignore-rules")
	outside := filepath.Join(temp, "outside-canary")
	if e = os.WriteFile(outside, []byte("XCLI_PUBLIC_CANARY"), 0600); e != nil {
		return out, e
	}
	link := filepath.Join(r.Workspace, "escape-canary")
	if e = os.Symlink(outside, link); e != nil {
		return out, e
	}
	defer os.Remove(link)
	var hits atomic.Int32
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return out, e
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); w.WriteHeader(204) }), ReadHeaderTimeout: time.Second}
	go server.Serve(listener)
	defer server.Close()
	tests := []struct {
		name    string
		command []string
		allow   bool
	}{
		{"read_selected", []string{"/bin/cat", filepath.Join(r.Workspace, "material.txt")}, true},
		{"deny_outside_read", []string{"/bin/cat", outside}, false},
		{"deny_workspace_write", []string{"/usr/bin/touch", filepath.Join(r.Workspace, "must-not-exist")}, false},
		{"deny_symlink_read", []string{"/bin/cat", link}, false},
		{"deny_tool_network", []string{"/usr/bin/curl", "--max-time", "1", "--noproxy", "*", "--silent", "http://" + listener.Addr().String() + "/canary"}, false},
	}
	for _, t := range tests {
		args := append([]string{"sandbox", "-P", "xcli_limited", "-C", r.Workspace}, LimitedPolicyArgs(r.Workspace)...)
		args = append(args, "--")
		args = append(args, t.command...)
		res, valid := run(t.name, args, Environment(nil, temp))
		out.Checks[t.name] = valid && ((res.ExitCode == 0) == t.allow)
		if t.name == "read_selected" {
			out.Checks[t.name] = out.Checks[t.name] && string(res.Stdout) == LimitedMaterial
		}
	}
	// A blanket sandbox startup failure must never count as successful denials.
	if !out.Checks["read_selected"] {
		for _, t := range tests {
			out.Checks[t.name] = false
		}
	}
	out.Checks["deny_tool_network"] = out.Checks["deny_tool_network"] && hits.Load() == 0
	_, err := os.Lstat(filepath.Join(r.Workspace, "must-not-exist"))
	out.Checks["deny_workspace_write"] = out.Checks["deny_workspace_write"] && os.IsNotExist(err)
	out.Checks["binary_unchanged"] = p.CheckBinary() == nil
	if e = limitedWrite(root, "codex-limited-probe.json", out); e != nil {
		return out, e
	}
	return out, nil
}
