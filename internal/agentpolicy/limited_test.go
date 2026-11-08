package agentpolicy

import (
	"context"
	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/modelstore"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func limitedFixture(t *testing.T) (string, LimitedRegistration) {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	script := filepath.Join(root, "fixture-codex")
	if e = os.WriteFile(script, []byte("#!/bin/sh\n[ \"$1\" = --version ] && [ \"$#\" = 1 ] || exit 91\nprintf 'codex-cli 0.167.0\\n'\n"), 0700); e != nil {
		t.Fatal(e)
	}
	state := filepath.Join(root, "state")
	if e = modelstore.PrivateDirectory(state); e != nil {
		t.Fatal(e)
	}
	r, e := PrepareLimited(context.Background(), state, script, root)
	if e != nil {
		t.Fatal(e)
	}
	return state, r
}
func TestLimitedDefaultAndCannotForgeNativeReadiness(t *testing.T) {
	root, r := limitedFixture(t)
	cfg := LimitedConfig{Enabled: true, ClientID: "owner", Purpose: "personal_local_synthetic", ProviderAuthorizationRef: "test-owner-assertion"}
	if e := EnableLimited(root, cfg); e == nil {
		t.Fatal("unverified native enabled")
	}
	if e := AuthorizeLimited(root, "new-call-reference", cfg); e == nil {
		t.Fatal("unverified call authorized")
	}
	if e := limitedWrite(root, "codex-limited-control.json", LimitedControl{RegistrationSHA256: api.Digest(r), Enabled: true}); e != nil {
		t.Fatal(e)
	}
	b := &LimitedBinding{Root: root, RegistrationSHA256: api.Digest(r), ClientID: "owner", Workspace: r.Workspace}
	if e := b.Check(true); e == nil {
		t.Fatal("forged control file promoted native")
	}
	if LimitedStatus(root, cfg).State != EngineeringBlocked {
		t.Fatal("unproven native readiness")
	}
	if e := DisableLimited(root); e != nil {
		t.Fatal(e)
	}
	if e := b.Check(false); e == nil {
		t.Fatal("disabled fixture executed")
	}
}
func TestLimitedWorkspaceAttacks(t *testing.T) {
	for _, attack := range []string{"symlink", "hardlink", "material", "extra-config", "git-hook", "root-symlink"} {
		t.Run(attack, func(t *testing.T) {
			root, r := limitedFixture(t)
			ws := r.Workspace
			switch attack {
			case "symlink":
				_ = os.Symlink(filepath.Join(root, "outside"), filepath.Join(ws, "escape"))
			case "hardlink":
				_ = os.Link(filepath.Join(ws, "material.txt"), filepath.Join(root, "material-alias"))
			case "material":
				_ = os.Chmod(filepath.Join(ws, "material.txt"), 0600)
				_ = os.WriteFile(filepath.Join(ws, "material.txt"), []byte("changed"), 0400)
			case "extra-config":
				_ = os.Mkdir(filepath.Join(ws, ".codex"), 0700)
			case "git-hook":
				_ = os.Mkdir(filepath.Join(ws, ".git", "hooks"), 0700)
			case "root-symlink":
				_ = os.Rename(ws, ws+"-moved")
				_ = os.Symlink(ws+"-moved", ws)
			}
			h, e := LimitedWorkspaceDigest(ws)
			if e == nil && h == r.WorkspaceSHA256 {
				t.Fatal("attack accepted", attack)
			}
		})
	}
}
func TestLimitedFixedArgumentsAndClosedConfiguration(t *testing.T) {
	args := LimitedExecArgs("/registered/workspace")
	joined := strings.Join(args, " ")
	for _, bad := range []string{"--full-auto", "--yolo", "--dangerously", "workspace-write", "danger-full-access", "auto_review", "--skip-git-repo-check"} {
		if strings.Contains(joined, bad) {
			t.Fatal(args)
		}
	}
	for _, want := range []string{"--ignore-user-config", "--ignore-rules", `approval_policy="never"`, `:root`, `network.enabled=false`} {
		if !strings.Contains(joined, want) {
			t.Fatal(args)
		}
	}
	for _, name := range []string{"shell_tool", "view_image", "plugins", "apps", "hooks", "multi_agent", "remote_plugin", "browser_use", "computer_use", "code_mode_host"} {
		if !strings.Contains(joined, "features."+name+"=false") {
			t.Fatal("missing candidate tool restriction", name)
		}
	}
	for _, want := range []string{`shell_environment_policy.inherit="none"`, `shell_environment_policy.ignore_default_excludes=false`} {
		if !strings.Contains(joined, want) {
			t.Fatal(want)
		}
	}
	cfg := DefaultConfig()
	if cfg.CodexLimited.Enabled {
		t.Fatal("enabled by default")
	}
	cfg.CodexLimited = LimitedConfig{Enabled: true, ClientID: "owner", Purpose: "shared_inference"}
	if cfg.Validate() == nil {
		t.Fatal("shared service allowed")
	}
}
