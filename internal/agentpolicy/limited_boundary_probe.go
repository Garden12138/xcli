package agentpolicy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	api "github.com/Garden12138/xcli/internal/agentapi"
	xr "github.com/Garden12138/xcli/internal/runtime"
)

// This probe never uses r.AuthHome. Both HOME and CODEX_HOME, every file and
// every endpoint are constructed canaries. No login, thread, turn or exec run.
type BoundaryObservation struct {
	Mechanism string `json:"mechanism"`
	Result    string `json:"result"`
	ExitCode  *int   `json:"exit_code,omitempty"`
}
type LimitedBoundaryProbe struct {
	Version             int                            `json:"version"`
	RegistrationSHA256  string                         `json:"registration_sha256"`
	PolicySHA256        string                         `json:"policy_sha256"`
	BinarySHA256        string                         `json:"binary_sha256"`
	Platform            string                         `json:"platform"`
	At                  time.Time                      `json:"at"`
	NativeProcesses     int                            `json:"native_processes"`
	ModelRequests       int                            `json:"model_requests"`
	RealCredentialsUsed bool                           `json:"real_credentials_used"`
	Observations        map[string]BoundaryObservation `json:"observations"`
}

func ProbeLimitedBoundary(ctx context.Context, root string) (LimitedBoundaryProbe, error) {
	out := LimitedBoundaryProbe{Version: 1, At: time.Now().UTC(), Platform: runtime.GOOS + "/" + runtime.GOARCH, Observations: map[string]BoundaryObservation{}}
	if runtime.GOOS != "darwin" {
		return out, errors.New("boundary_probe_platform_unverified")
	}
	r, e := ReadLimited(root)
	if e != nil {
		return out, e
	}
	p := Profile{Binary: r.Binary, BinarySHA256: r.BinarySHA256}
	if e = p.CheckBinary(); e != nil {
		return out, e
	}
	if CheckLimitedWorkspace(r) != nil {
		return out, errors.New("boundary_workspace_changed")
	}
	out.RegistrationSHA256 = api.Digest(r)
	out.PolicySHA256 = api.Digest(LimitedExecArgs(r.Workspace))
	out.BinarySHA256 = r.BinarySHA256
	temp, e := os.MkdirTemp(filepath.Join(root, "codex-limited"), "boundary-")
	if e != nil {
		return out, e
	}
	defer os.RemoveAll(temp)
	fakeHome := filepath.Join(temp, "fake-home")
	fakeAuth := filepath.Join(temp, "fake-auth")
	for _, d := range []string{fakeHome, fakeAuth} {
		if e = os.Mkdir(d, 0700); e != nil {
			return out, e
		}
	}
	const fake = "XCLI_FAKE_CREDENTIAL_CANARY_NOT_A_LOGIN\n"
	credential := filepath.Join(fakeAuth, "auth.json")
	if e = os.WriteFile(credential, []byte(fake), 0600); e != nil {
		return out, e
	}
	outside := filepath.Join(temp, "outside.txt")
	if e = os.WriteFile(outside, []byte("XCLI_OUTSIDE_CANARY\n"), 0600); e != nil {
		return out, e
	}
	env := append(Environment(nil, fakeHome), "CODEX_HOME="+fakeAuth)
	observe := func(name, mechanism string, pass bool, code *int) {
		result := "FAIL"
		if pass {
			result = "PASS"
		}
		out.Observations[name] = BoundaryObservation{mechanism, result, code}
	}
	run := func(command string, args []string, native bool) (xr.ProcessResult, bool) {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if native {
			out.NativeProcesses++
		}
		res, err := xr.RunProcess(c, agent.CommandSpec{Command: command, Args: args}, xr.ProcessOptions{Dir: r.Workspace, Env: env, CaptureStdout: true, SeparateProcess: true, BoundedCleanup: true})
		return res, err == nil && res.StopConfirmed && !res.TimedOut && !res.Canceled && !res.CaptureTruncated && !res.IOError && len(res.Stdout) <= 64<<10
	}
	control, valid := run("/bin/cat", []string{credential}, false)
	observe("same_uid_unsandboxed_fake_credential_readable", "OS_PROCESS_CONTROL", valid && control.ExitCode == 0 && string(control.Stdout) == fake, &control.ExitCode)
	// Path hiding alone cannot prevent an absolute-path read under the same UID.
	sanitized := Environment([]string{"OPENAI_API_KEY=FAKE", "CODEX_HOME=FAKE", "HTTP_PROXY=FAKE", "DYLD_INSERT_LIBRARIES=FAKE", "XCLI_APP_TOKEN=FAKE"}, fakeHome)
	observe("gateway_native_environment_allowlist", "GO_ALLOWLIST", len(sanitized) == 3 && !strings.Contains(strings.Join(sanitized, "\n"), "FAKE"), nil)
	var hits atomic.Int32
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return out, e
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); w.WriteHeader(204) }), ReadHeaderTimeout: time.Second}
	go server.Serve(listener)
	defer server.Close()
	endpoint := "http://" + listener.Addr().String() + "/canary"
	baseline, valid := run("/usr/bin/curl", []string{"--silent", "--max-time", "1", "--noproxy", "*", endpoint}, false)
	observe("parent_can_connect_mock_model_service", "OS_PROCESS_CONTROL", valid && baseline.ExitCode == 0 && hits.Load() == 1, &baseline.ExitCode)
	beforeHits := hits.Load()
	link := filepath.Join(r.Workspace, "boundary-escape")
	if e = os.Symlink(credential, link); e != nil {
		return out, e
	}
	defer os.Remove(link)
	rel, _ := filepath.Rel(r.Workspace, credential)
	attempts := []struct {
		name    string
		command []string
		allowed bool
	}{
		{"selected_read", []string{"/bin/cat", filepath.Join(r.Workspace, "material.txt")}, true},
		{"same_uid", []string{"/usr/bin/id", "-u"}, true},
		{"fake_auth_absolute_read_denied", []string{"/bin/cat", credential}, false},
		{"fake_auth_relative_read_denied", []string{"/bin/cat", rel}, false},
		{"outside_read_denied", []string{"/bin/cat", outside}, false},
		{"symlink_fake_auth_read_denied", []string{"/bin/cat", link}, false},
		{"workspace_write_denied", []string{"/usr/bin/touch", filepath.Join(r.Workspace, "boundary-must-not-exist")}, false},
		{"mock_service_tool_network_denied", []string{"/usr/bin/curl", "--silent", "--max-time", "1", "--noproxy", "*", endpoint}, false},
	}
	positive := false
	for _, test := range attempts {
		args := append([]string{"sandbox", "-P", "xcli_limited", "-C", r.Workspace}, LimitedPolicyArgs(r.Workspace)...)
		args = append(args, "--")
		args = append(args, test.command...)
		res, ok := run(r.Binary, args, true)
		pass := ok && ((res.ExitCode == 0) == test.allowed)
		if test.name == "selected_read" {
			pass = pass && string(res.Stdout) == LimitedMaterial
			positive = pass
		}
		if test.name == "same_uid" {
			pass = pass && strings.TrimSpace(string(res.Stdout)) == strconv.Itoa(os.Geteuid())
		}
		if !test.allowed {
			pass = pass && positive
		}
		if test.name == "mock_service_tool_network_denied" {
			pass = pass && hits.Load() == beforeHits
		}
		if test.name == "workspace_write_denied" {
			_, statErr := os.Lstat(filepath.Join(r.Workspace, "boundary-must-not-exist"))
			pass = pass && os.IsNotExist(statErr)
		}
		observe(test.name, "NATIVE_SANDBOX_OS_CANARY", pass, &res.ExitCode)
	}
	featureArgs := []string{"features", "list"}
	disabled := []string{"shell_tool", "view_image", "plugins", "apps", "hooks", "multi_agent", "remote_plugin", "browser_use", "computer_use", "code_mode_host"}
	for _, name := range disabled {
		featureArgs = append(featureArgs, "-c", "features."+name+"=false")
	}
	features, ok := run(r.Binary, featureArgs, true)
	effective := map[string]string{}
	for _, line := range strings.Split(string(features.Stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			effective[fields[0]] = fields[len(fields)-1]
		}
	}
	for _, name := range disabled {
		observe("feature_disabled_"+name, "OFFICIAL_FEATURE_RESOLVER_NOT_TOOL_CATALOG", ok && features.ExitCode == 0 && effective[name] == "false", &features.ExitCode)
	}
	out.Observations["exec_effective_tool_catalog"] = BoundaryObservation{Mechanism: "REAL_EXEC_TURN_NOT_RUN", Result: "NOT_PROVEN"}
	out.Observations["direct_builtin_credential_io"] = BoundaryObservation{Mechanism: "NO_INDEPENDENT_BROKER_OR_OS_IDENTITY", Result: "NOT_PROVEN"}
	out.Observations["real_model_service_connectivity"] = BoundaryObservation{Mechanism: "MOCK_ENDPOINT_ONLY_NO_PROVIDER_CALL", Result: "NOT_TESTED"}
	out.Observations["exec_tool_environment_filter"] = BoundaryObservation{Mechanism: "CONFIG_STATIC_ONLY_SANDBOX_HELPER_IS_NOT_EXEC_TOOL_LAUNCHER", Result: "NOT_PROVEN"}
	observe("binary_unchanged", "FILE_DIGEST", p.CheckBinary() == nil, nil)
	if e = os.Remove(link); e != nil {
		return out, e
	}
	observe("workspace_unchanged", "DIRECTORY_IDENTITY_NOFOLLOW_FD_AND_DIGEST", CheckLimitedWorkspace(r) == nil, nil)
	if e = limitedWrite(root, "codex-limited-boundary.json", out); e != nil {
		return out, e
	}
	return out, nil
}
