package agentpolicy

// The limited control plane is separate from inference authorization. Its
// native boundary verifier deliberately cannot promote an unproven tool route.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentartifact"
	"github.com/Garden12138/xcli/internal/modelstore"
	xr "github.com/Garden12138/xcli/internal/runtime"
)

const LimitedPolicy = "codex-limited-readonly-v1"
const LimitedMaterial = "XCLI_G5B_CODEX_SYNTHETIC_20261108\n"
const LimitedPrompt = "Return exactly XCLI_G5B_CODEX_SYNTHETIC_20261108 and nothing else. Do not use tools."

type LimitedConfig struct {
	Enabled                  bool   `yaml:"enabled" json:"enabled"`
	ClientID                 string `yaml:"client_id" json:"client_id"`
	Purpose                  string `yaml:"purpose" json:"purpose"`
	ProviderAuthorizationRef string `yaml:"provider_authorization_ref" json:"provider_authorization_ref"`
}

func (c LimitedConfig) Validate() error {
	if c == (LimitedConfig{}) {
		return nil
	}
	if c.ClientID == "" || len(c.ClientID) > 64 || strings.ContainsAny(c.ClientID, "/\\\x00\r\n") || c.Purpose != "personal_local_synthetic" || len(c.ProviderAuthorizationRef) > 128 {
		return errors.New("invalid codex_limited configuration")
	}
	return nil
}

type LimitedRegistration struct {
	Version         int    `json:"version"`
	Binary          string `json:"binary"`
	BinarySHA256    string `json:"binary_sha256"`
	CLIVersion      string `json:"cli_version"`
	AuthHome        string `json:"auth_home"`
	Workspace       string `json:"workspace"`
	WorkspaceSHA256 string `json:"workspace_sha256"`
}
type LimitedControl struct {
	RegistrationSHA256 string    `json:"registration_sha256"`
	Enabled            bool      `json:"enabled"`
	AuthorizationRef   string    `json:"authorization_ref"`
	ExpiresAt          time.Time `json:"expires_at"`
}
type LimitedBinding struct {
	Root               string
	RegistrationSHA256 string
	ClientID           string
	Workspace          string
	AuthHome           string
}
type LimitedReadiness struct {
	State              string `json:"state"`
	RegistrationSHA256 string `json:"registration_sha256"`
	Checks             []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"checks"`
	ModelRequests          int    `json:"model_requests"`
	Enabled                bool   `json:"enabled"`
	CallAuthorized         bool   `json:"call_authorized"`
	AuthorizationRefSHA256 string `json:"authorization_ref_sha256"`
	ClientReady            bool   `json:"client_ready"`
}

func limitedRead(root, name string, v any) error {
	b, e := modelstore.ReadPrivate(filepath.Join(root, name), 1<<20)
	if e != nil {
		return errors.New("limited_state_unavailable")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return errors.New("limited_state_invalid")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("limited_state_invalid")
	}
	return nil
}
func limitedWrite(root, name string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if e = modelstore.WritePrivate(filepath.Join(root, name), b); e != nil {
		return e
	}
	d, e := os.Open(root)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func ReadLimited(root string) (LimitedRegistration, error) {
	var r LimitedRegistration
	e := limitedRead(root, "codex-limited-registration.json", &r)
	if e == nil && (r.Version != 1 || r.Workspace != filepath.Join(root, "codex-limited", "workspace")) {
		e = errors.New("limited_registration_invalid")
	}
	return r, e
}
func LimitedPolicyArgs(workspace string) []string {
	fs := `permissions.xcli_limited.filesystem={":root"="deny",":minimal"="read",":tmpdir"="deny",":slash_tmp"="deny",` + strconv.Quote(workspace) + `="read"}`
	return []string{"-c", `approval_policy="never"`, "-c", `default_permissions="xcli_limited"`, "-c", fs, "-c", "permissions.xcli_limited.network.enabled=false"}
}
func LimitedExecArgs(workspace string) []string {
	out := []string{"exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules"}
	out = append(out, LimitedPolicyArgs(workspace)...)
	for _, v := range []string{`mcp_servers={}`, `web_search="disabled"`, `plugins._default.enabled=false`, `apps._default.enabled=false`, `features.apps=false`, `features.hooks=false`, `features.multi_agent=false`, `features.remote_plugin=false`, `features.shell_snapshot=false`, `allow_login_shell=false`} {
		out = append(out, "-c", v)
	}
	return out
}

// Only creates a new, service-owned synthetic project; never imports a repo.
func PrepareLimited(ctx context.Context, root, binary, authHome string) (LimitedRegistration, error) {
	var r LimitedRegistration
	if !filepath.IsAbs(authHome) || strings.ContainsAny(authHome, "\x00\r\n") {
		return r, errors.New("absolute_native_auth_home_required")
	}
	ah, e := filepath.EvalSymlinks(authHome)
	if e != nil || ah != filepath.Clean(authHome) {
		return r, errors.New("canonical_native_auth_home_required")
	}
	if _, e = os.Lstat(filepath.Join(root, "codex-limited-registration.json")); !os.IsNotExist(e) {
		return r, errors.New("limited_registration_already_exists")
	}
	cached, e := Refresh(ctx, root, "codex", binary)
	if e != nil {
		return r, e
	}
	if cached.Version != "codex-cli 0.167.0" {
		return r, errors.New("codex_version_unverified")
	}
	base := filepath.Join(root, "codex-limited")
	if e = modelstore.PrivateDirectory(base); e != nil {
		return r, e
	}
	ws := filepath.Join(base, "workspace")
	if e = os.Mkdir(ws, 0700); e != nil {
		return r, e
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(ws)
		}
	}()
	if e = os.WriteFile(filepath.Join(ws, "material.txt"), []byte(LimitedMaterial), 0400); e != nil {
		return r, e
	}
	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	proc, e := xr.RunProcess(pctx, agent.CommandSpec{Command: "/usr/bin/git", Args: []string{"-c", "core.hooksPath=/dev/null", "init", "--template=", "--quiet", ws}}, xr.ProcessOptions{Dir: ws, Env: append(Environment(nil, base), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"), SeparateProcess: true, BoundedCleanup: true})
	if e != nil || proc.ExitCode != 0 || !proc.StopConfirmed {
		return r, errors.New("synthetic_git_init_failed")
	}
	// Fix the complete allowlisted Git file set; extra files/links invalidate it.
	hash, e := LimitedWorkspaceDigest(ws)
	if e != nil {
		return r, e
	}
	r = LimitedRegistration{1, cached.Binary, cached.BinarySHA256, cached.Version, ah, ws, hash}
	if e = limitedWrite(root, "codex-limited-registration.json", r); e != nil {
		return r, e
	}
	e = limitedWrite(root, "codex-limited-control.json", LimitedControl{RegistrationSHA256: api.Digest(r)})
	ok = e == nil
	return r, e
}
func LimitedWorkspaceDigest(ws string) (string, error) {
	info, e := os.Lstat(ws)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("limited_workspace_invalid")
	}
	canonical, e := filepath.EvalSymlinks(ws)
	if e != nil || canonical != filepath.Clean(ws) {
		return "", errors.New("limited_workspace_symlink")
	}
	allowed := []string{"material.txt", ".git/HEAD", ".git/config"}
	files := map[string]string{}
	e = filepath.WalkDir(ws, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(ws, path)
		if rel == "." {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("limited_workspace_symlink")
		}
		if d.IsDir() {
			if !slices.Contains([]string{".git", ".git/refs", ".git/refs/heads", ".git/refs/tags", ".git/objects", ".git/objects/info", ".git/objects/pack"}, filepath.ToSlash(rel)) {
				return errors.New("unexpected_workspace_directory")
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !slices.Contains(allowed, rel) {
			return errors.New("unexpected_workspace_file")
		}
		b, e := agentartifact.ReadBound(ws, rel, 64<<10)
		if e != nil {
			return e
		}
		files[rel] = api.Hash(b)
		return nil
	})
	if e != nil {
		return "", e
	}
	if len(files) != len(allowed) || files["material.txt"] != api.Hash([]byte(LimitedMaterial)) {
		return "", errors.New("limited_material_changed")
	}
	return api.Digest(files), nil
}

// Complete builtin-tool/config/credential isolation is NOT inferred from a
// shell canary. This fail-closed verifier cannot be overridden by JSON/YAML.
func nativeToolBoundaryVerified() bool { return false }
func LimitedStatus(root string, cfg LimitedConfig) LimitedReadiness {
	out := LimitedReadiness{State: "BLOCKED"}
	add := func(id, status string) {
		out.Checks = append(out.Checks, struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}{id, status})
	}
	r, e := ReadLimited(root)
	if e != nil {
		add("registration", "BLOCKED")
		return out
	}
	out.RegistrationSHA256 = api.Digest(r)
	var control LimitedControl
	if limitedRead(root, "codex-limited-control.json", &control) == nil && control.RegistrationSHA256 == out.RegistrationSHA256 {
		out.Enabled = cfg.Enabled && control.Enabled
		out.CallAuthorized = out.Enabled && control.AuthorizationRef != "" && control.ExpiresAt.After(time.Now())
		if control.AuthorizationRef != "" {
			out.AuthorizationRefSHA256 = api.Hash([]byte(control.AuthorizationRef))
		}
	}
	add("registration", "PASS")
	ws, e := LimitedWorkspaceDigest(r.Workspace)
	if e == nil && ws == r.WorkspaceSHA256 {
		add("workspace", "PASS")
	} else {
		add("workspace", "BLOCKED")
	}
	p := Profile{Binary: r.Binary, BinarySHA256: r.BinarySHA256}
	if p.CheckBinary() == nil {
		add("binary_integrity", "PASS")
	} else {
		add("binary_integrity", "BLOCKED")
	}
	if cfg.ProviderAuthorizationRef != "" {
		add("provider_use_case", "OWNER_ASSERTED_NOT_PROVIDER_VERIFIED")
	} else {
		add("provider_use_case", "BLOCKED")
	}
	var probe LimitedProbe
	if limitedRead(root, "codex-limited-probe.json", &probe) == nil && probe.RegistrationSHA256 == out.RegistrationSHA256 && probe.PolicySHA256 == api.Digest(LimitedExecArgs(r.Workspace)) && time.Since(probe.At) >= 0 && time.Since(probe.At) < 15*time.Minute {
		add("native_login", probe.Login)
		pass := probe.Checks["binary_unchanged"] && probe.Checks["fixed_command_format"]
		for _, id := range []string{"read_selected", "deny_outside_read", "deny_symlink_read", "deny_workspace_write", "deny_tool_network"} {
			pass = pass && probe.Checks[id]
		}
		if pass {
			add("native_sandbox", "PASS_COMMAND_CANARIES_ONLY")
		} else {
			add("native_sandbox", "BLOCKED")
		}
	} else {
		add("native_login", "RUN_EXPLICIT_PROBE")
		add("native_sandbox", "RUN_EXPLICIT_PROBE")
	}
	add("builtin_tools_and_auth_isolation", "BLOCKED")
	if nativeToolBoundaryVerified() {
		out.State = "READY_FOR_AUTHORIZATION"
	}
	return out
}
func (b *LimitedBinding) Check(native bool) error {
	var c LimitedControl
	if limitedRead(b.Root, "codex-limited-control.json", &c) != nil || !c.Enabled || c.RegistrationSHA256 != b.RegistrationSHA256 {
		return api.Failure(503, "policy_unenforceable")
	}
	r, e := ReadLimited(b.Root)
	if e != nil || api.Digest(r) != b.RegistrationSHA256 {
		return api.Failure(409, "profile_changed")
	}
	h, e := LimitedWorkspaceDigest(b.Workspace)
	if e != nil || h != r.WorkspaceSHA256 {
		return api.Failure(503, "policy_unenforceable")
	}
	if native && !nativeToolBoundaryVerified() {
		return api.Failure(503, "policy_unenforceable")
	}
	return nil
}
func (b *LimitedBinding) CheckAuthorization() error {
	var c LimitedControl
	if limitedRead(b.Root, "codex-limited-control.json", &c) != nil || c.AuthorizationRef == "" || !c.ExpiresAt.After(time.Now()) {
		return api.Failure(403, "policy_denied")
	}
	return nil
}
func (b *LimitedBinding) AuthorizationSHA256() string {
	var c LimitedControl
	if limitedRead(b.Root, "codex-limited-control.json", &c) != nil || c.RegistrationSHA256 != b.RegistrationSHA256 || c.AuthorizationRef == "" {
		return ""
	}
	return api.Hash([]byte(c.AuthorizationRef))
}
func DisableLimited(root string) error {
	lock, e := LockLimited(root)
	if e != nil {
		return e
	}
	defer lock.Close()
	var c LimitedControl
	if e := limitedRead(root, "codex-limited-control.json", &c); e != nil {
		return e
	}
	c.Enabled = false
	c.AuthorizationRef = ""
	c.ExpiresAt = time.Time{}
	return limitedWrite(root, "codex-limited-control.json", c)
}
func EnableLimited(root string, cfg LimitedConfig) error {
	lock, e := LockLimited(root)
	if e != nil {
		return e
	}
	defer lock.Close()
	if !cfg.Enabled || cfg.ProviderAuthorizationRef == "" || LimitedStatus(root, cfg).State != "READY_FOR_AUTHORIZATION" {
		return errors.New("limited_profile_not_ready")
	}
	r, e := ReadLimited(root)
	if e != nil {
		return e
	}
	return limitedWrite(root, "codex-limited-control.json", LimitedControl{RegistrationSHA256: api.Digest(r), Enabled: true})
}
func AuthorizeLimited(root, ref string) error {
	lock, e := LockLimited(root)
	if e != nil {
		return e
	}
	defer lock.Close()
	var c LimitedControl
	if limitedRead(root, "codex-limited-control.json", &c) != nil || !c.Enabled || !nativeToolBoundaryVerified() || ref == "" || len(ref) > 128 {
		return errors.New("limited_profile_not_ready_for_call_authorization")
	}
	c.AuthorizationRef = ref
	c.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	return limitedWrite(root, "codex-limited-control.json", c)
}

func LockLimited(root string) (*os.File, error) {
	return modelstore.Lock(filepath.Join(root, "codex-limited.lock"), true)
}
func BindLimited(root string, cfg Config, profiles []Profile) ([]Profile, error) {
	if !cfg.CodexLimited.Enabled {
		return profiles, nil
	}
	r, e := ReadLimited(root)
	if e != nil {
		return nil, e
	}
	for i, p := range profiles {
		if p.AgentID == "codex" {
			p.PolicyID = LimitedPolicy
			p.Operations = []string{"text_task"}
			p.Binary = r.Binary
			p.BinarySHA256 = r.BinarySHA256
			p.Version = &r.CLIVersion
			p.Blocker = ""
			p.Limited = &LimitedBinding{root, api.Digest(r), cfg.CodexLimited.ClientID, r.Workspace, r.AuthHome}
			profiles[i] = p.Freeze()
		}
	}
	return profiles, nil
}
