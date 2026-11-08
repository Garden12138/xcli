package agentpolicy

// The limited control plane is separate from inference authorization. Its
// native boundary verifier deliberately cannot promote an unproven tool route.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	Version           int    `json:"version"`
	Binary            string `json:"binary"`
	BinarySHA256      string `json:"binary_sha256"`
	CLIVersion        string `json:"cli_version"`
	AuthHome          string `json:"auth_home"`
	Workspace         string `json:"workspace"`
	WorkspaceSHA256   string `json:"workspace_sha256"`
	WorkspaceIdentity string `json:"workspace_identity,omitempty"`
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
	State                  string   `json:"state"`
	EngineeringState       string   `json:"engineering_state"`
	UserConfirmationState  string   `json:"user_confirmation_state"`
	UserConfirmationSHA256 string   `json:"user_confirmation_sha256"`
	Blockers               []string `json:"blockers"`
	RegistrationSHA256     string   `json:"registration_sha256"`
	Checks                 []struct {
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
	if strictLimitedDecode(b, v) != nil {
		return errors.New("limited_state_invalid")
	}
	return nil
}
func strictLimitedDecode(b []byte, v any) error {
	// Validate duplicate keys/UTF-8 before typed decoding. Private state includes
	// time.Time, unlike wire DTOs, so wire requiredShape is applied to a raw map.
	var shape map[string]json.RawMessage
	if e := api.StrictDecode(b, &shape); e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(v)
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
	for _, v := range []string{`mcp_servers={}`, `web_search="disabled"`, `plugins._default.enabled=false`, `apps._default.enabled=false`, `features.apps=false`, `features.hooks=false`, `features.multi_agent=false`, `features.remote_plugin=false`, `features.shell_snapshot=false`, `features.shell_tool=false`, `features.view_image=false`, `features.plugins=false`, `features.browser_use=false`, `features.computer_use=false`, `features.code_mode_host=false`, `shell_environment_policy.inherit="none"`, `shell_environment_policy.ignore_default_excludes=false`, `allow_login_shell=false`} {
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
	identity, e := limitedDirectoryIdentity(ws)
	if e != nil {
		return r, e
	}
	r = LimitedRegistration{Version: 1, Binary: cached.Binary, BinarySHA256: cached.BinarySHA256, CLIVersion: cached.Version, AuthHome: ah, Workspace: ws, WorkspaceSHA256: hash, WorkspaceIdentity: identity}
	if e = limitedWrite(root, "codex-limited-registration.json", r); e != nil {
		return r, e
	}
	e = limitedWrite(root, "codex-limited-control.json", LimitedControl{RegistrationSHA256: api.Digest(r)})
	ok = e == nil
	return r, e
}

// The digest alone cannot distinguish an identical-content directory swap.
// Legacy registrations lack identity and must be explicitly prepared afresh.
func CheckLimitedWorkspace(r LimitedRegistration) error {
	identity, e := limitedDirectoryIdentity(r.Workspace)
	if e != nil || r.WorkspaceIdentity == "" || identity != r.WorkspaceIdentity {
		return errors.New("limited_workspace_identity_changed")
	}
	h, e := LimitedWorkspaceDigest(r.Workspace)
	if e != nil || h != r.WorkspaceSHA256 {
		return errors.New("limited_workspace_changed")
	}
	return nil
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

func LimitedStatus(root string, cfg LimitedConfig) LimitedReadiness {
	out := LimitedReadiness{State: EngineeringBlocked, EngineeringState: EngineeringBlocked, UserConfirmationState: UserConfirmationRequired, Blockers: []string{}}
	add := func(id, status string) {
		out.Checks = append(out.Checks, struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}{id, status})
	}
	r, e := ReadLimited(root)
	if e != nil {
		add("registration", "BLOCKED")
		out.Blockers = append(out.Blockers, "registration_unavailable")
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
	workspaceErr := CheckLimitedWorkspace(r)
	if workspaceErr == nil {
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
	var probe LimitedProbe
	loginReady := false
	if limitedRead(root, "codex-limited-probe.json", &probe) == nil && probe.RegistrationSHA256 == out.RegistrationSHA256 && probe.PolicySHA256 == api.Digest(LimitedExecArgs(r.Workspace)) && time.Since(probe.At) >= 0 && time.Since(probe.At) < 15*time.Minute {
		add("native_login", probe.Login)
		loginReady = probe.Login == "AUTHENTICATED_CHATGPT"
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
		add("native_login", "NOT_TESTED_CURRENT_POLICY")
		add("native_sandbox", "RUN_EXPLICIT_PROBE")
	}
	confirmed, digest := userConfirmation(root, r, cfg)
	out.UserConfirmationSHA256 = digest
	if confirmed {
		add("provider_use_case", "OWNER_CONFIRMED_NOT_PROVIDER_ENTITLEMENT_PROOF")
	} else {
		add("provider_use_case", UserConfirmationRequired)
	}
	var boundary LimitedBoundaryProbe
	boundaryReady := nativeBoundaryVerified(root, r)
	if limitedRead(root, "codex-limited-boundary.json", &boundary) == nil && boundaryProbeValid(boundary, r) {
		add("native_command_credential_canaries", "SCOPED_OS_EVIDENCE_AVAILABLE")
	} else {
		out.Blockers = append(out.Blockers, "current_boundary_probe_required")
	}
	out.Blockers = append(out.Blockers, execBoundaryMissing()...)
	if !loginReady {
		out.Blockers = append(out.Blockers, "native_login_status_not_current")
	}
	if workspaceErr != nil {
		out.Blockers = append(out.Blockers, "workspace_identity_or_material_invalid")
	}
	if p.CheckBinary() != nil {
		out.Blockers = append(out.Blockers, "binary_integrity_invalid")
	}
	engineering := boundaryReady && loginReady && p.CheckBinary() == nil && workspaceErr == nil
	out.State, out.EngineeringState, out.UserConfirmationState = resolveLimitedState(engineering, confirmed)
	add("builtin_tools_and_auth_isolation", out.EngineeringState)
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
	if b.Workspace != r.Workspace || CheckLimitedWorkspace(r) != nil {
		return api.Failure(503, "policy_unenforceable")
	}
	confirmed, _ := userConfirmation(b.Root, r, LimitedConfig{ClientID: b.ClientID})
	if native && (!nativeBoundaryVerified(b.Root, r) || !confirmed) {
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
	if !cfg.Enabled || LimitedStatus(root, cfg).State != ReadyForAuthorization {
		return errors.New("limited_profile_not_ready")
	}
	r, e := ReadLimited(root)
	if e != nil {
		return e
	}
	return limitedWrite(root, "codex-limited-control.json", LimitedControl{RegistrationSHA256: api.Digest(r), Enabled: true})
}
func AuthorizeLimited(root, ref string, cfg LimitedConfig) error {
	lock, e := LockLimited(root)
	if e != nil {
		return e
	}
	defer lock.Close()
	var c LimitedControl
	r, err := ReadLimited(root)
	confirmed, _ := userConfirmation(root, r, cfg)
	if limitedRead(root, "codex-limited-control.json", &c) != nil || !cfg.Enabled || !c.Enabled || err != nil || c.RegistrationSHA256 != api.Digest(r) || !confirmed || !nativeBoundaryVerified(root, r) || ref == "" || len(ref) > 128 {
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
