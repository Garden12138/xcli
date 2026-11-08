package agentpolicy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/modelstore"
)

func TestLimitedIndependentReadinessStates(t *testing.T) {
	for _, tt := range []struct {
		eng, user                        bool
		state, engineering, confirmation string
	}{
		{false, false, EngineeringBlocked, EngineeringBlocked, UserConfirmationRequired},
		{false, true, EngineeringBlocked, EngineeringBlocked, "CONFIRMED"},
		{true, false, UserConfirmationRequired, EngineeringReady, UserConfirmationRequired},
		{true, true, ReadyForAuthorization, EngineeringReady, "CONFIRMED"},
	} {
		a, b, c := resolveLimitedState(tt.eng, tt.user)
		if a != tt.state || b != tt.engineering || c != tt.confirmation {
			t.Fatal(a, b, c, tt)
		}
	}
}

func TestLimitedOwnerConfirmationCannotPromoteEngineeringOrAuthorize(t *testing.T) {
	root, r := limitedFixture(t)
	cfg := LimitedConfig{Enabled: true, ClientID: "owner", Purpose: "personal_local_synthetic", ProviderAuthorizationRef: "old-YAML-is-not-consent"}
	c, e := LimitedConfirmationTemplate(root, cfg)
	if e != nil {
		t.Fatal(e)
	}
	if ConfirmLimited(root, cfg, c) == nil {
		t.Fatal("all-false template accepted")
	}
	if confirmed, _ := userConfirmation(root, r, cfg); confirmed {
		t.Fatal("inferred from YAML")
	}
	// All declarations here refer only to an offline unit-test owner fixture.
	c.Reference = "unit-test-only"
	c.OwnAuthorizedAccount = true
	c.PlanAndCapabilityAvailable = true
	c.PersonalLocalDevelopmentOnly = true
	c.NoSharingCommercialOrHosting = true
	c.AllowSyntheticCloudRequest = true
	c.UnderstandInternalCallsAndCost = true
	b, _ := json.Marshal(c)
	for _, bad := range []string{strings.TrimSuffix(string(b), "}") + `,"unexpected":true}`, strings.TrimSuffix(string(b), "}") + `,"allow_synthetic_cloud_request":false}`, `{}`, `null`} {
		if ConfirmLimitedJSON(root, cfg, []byte(bad)) == nil {
			t.Fatal("invalid or ambiguous owner import accepted")
		}
	}
	if e = ConfirmLimitedJSON(root, cfg, b); e != nil {
		t.Fatal(e)
	}
	var saved LimitedConfirmation
	if e = limitedRead(root, "codex-limited-confirmation.json", &saved); e != nil {
		t.Fatal(e)
	}
	if !confirmationValid(saved, r, cfg) {
		t.Fatal("current fixture confirmation invalid")
	}
	for _, attack := range []string{"account", "plan", "use", "sharing", "cloud", "fees", "client", "registration", "policy", "expired", "future", "email"} {
		v := saved
		switch attack {
		case "account":
			v.OwnAuthorizedAccount = false
		case "plan":
			v.PlanAndCapabilityAvailable = false
		case "use":
			v.PersonalLocalDevelopmentOnly = false
		case "sharing":
			v.NoSharingCommercialOrHosting = false
		case "cloud":
			v.AllowSyntheticCloudRequest = false
		case "fees":
			v.UnderstandInternalCallsAndCost = false
		case "client":
			v.ClientID = "other"
		case "registration":
			v.RegistrationSHA256 = "old"
		case "policy":
			v.PolicySHA256 = "old"
		case "expired":
			v.RecordedAt = time.Now().Add(-25 * time.Hour)
		case "future":
			v.RecordedAt = time.Now().Add(time.Minute)
		case "email":
			v.Reference = "no-real-email@example.invalid"
		}
		if confirmationValid(v, r, cfg) {
			t.Fatal("accepted", attack)
		}
	}
	status := LimitedStatus(root, cfg)
	if status.EngineeringState != EngineeringBlocked || status.UserConfirmationState != "CONFIRMED" || status.State != EngineeringBlocked || status.Enabled || status.CallAuthorized {
		t.Fatal(status)
	}
	if EnableLimited(root, cfg) == nil || AuthorizeLimited(root, "unit-call", cfg) == nil {
		t.Fatal("confirmation bypassed engineering")
	}
	if e = DisableLimited(root); e != nil {
		t.Fatal(e)
	}
}

func TestLimitedForgedBoundaryReceiptCannotAddMissingMechanisms(t *testing.T) {
	root, r := limitedFixture(t)
	p := LimitedBoundaryProbe{Version: 1, RegistrationSHA256: api.Digest(r), PolicySHA256: api.Digest(LimitedExecArgs(r.Workspace)), BinarySHA256: r.BinarySHA256, At: time.Now().UTC(), Observations: map[string]BoundaryObservation{}}
	for _, name := range []string{"selected_read", "fake_auth_absolute_read_denied", "fake_auth_relative_read_denied", "outside_read_denied", "symlink_fake_auth_read_denied", "workspace_write_denied", "mock_service_tool_network_denied", "workspace_unchanged", "binary_unchanged", "exec_effective_tool_catalog", "direct_builtin_credential_io", "exec_tool_environment_filter"} {
		p.Observations[name] = BoundaryObservation{Result: "PASS", Mechanism: "FORGED_UNIT_FIXTURE"}
	}
	if !boundaryProbeValid(p, r) {
		t.Fatal("fixture binding invalid")
	}
	for _, attack := range []string{"expired", "policy", "registration", "credentials", "model", "binary"} {
		v := p
		switch attack {
		case "expired":
			v.At = time.Now().Add(-16 * time.Minute)
		case "policy":
			v.PolicySHA256 = "old"
		case "registration":
			v.RegistrationSHA256 = "old"
		case "credentials":
			v.RealCredentialsUsed = true
		case "model":
			v.ModelRequests = 1
		case "binary":
			v.BinarySHA256 = "other"
		}
		if boundaryProbeValid(v, r) {
			t.Fatal(attack)
		}
	}
	if e := limitedWrite(root, "codex-limited-boundary.json", p); e != nil {
		t.Fatal(e)
	}
	if nativeBoundaryVerified(root, r) {
		t.Fatal("receipt created missing IO broker/tool attestation")
	}
	if len(LimitedStatus(root, LimitedConfig{}).Blockers) < 3 {
		t.Fatal("missing independent blockers")
	}
}

func TestLimitedIdenticalDirectoryReplacementAndLegacyRegistration(t *testing.T) {
	root, r := limitedFixture(t)
	legacy := r
	legacy.WorkspaceIdentity = ""
	if CheckLimitedWorkspace(legacy) == nil {
		t.Fatal("legacy silently bound new directory")
	}
	moved := r.Workspace + "-original"
	if e := os.Rename(r.Workspace, moved); e != nil {
		t.Fatal(e)
	}
	if e := os.CopyFS(r.Workspace, os.DirFS(moved)); e != nil {
		t.Fatal(e)
	}
	h, e := LimitedWorkspaceDigest(r.Workspace)
	if e != nil || h != r.WorkspaceSHA256 {
		t.Fatal("copy must have identical allowed bytes", e)
	}
	if CheckLimitedWorkspace(r) == nil {
		t.Fatal("identical-content directory replacement accepted")
	}
	if e = limitedWrite(root, "codex-limited-control.json", LimitedControl{RegistrationSHA256: api.Digest(r), Enabled: true}); e != nil {
		t.Fatal(e)
	}
	binding := LimitedBinding{Root: root, RegistrationSHA256: api.Digest(r), ClientID: "owner", Workspace: r.Workspace}
	if binding.Check(false) == nil {
		t.Fatal("replacement dispatched")
	}
}

func TestLimitedPrivateStateRejectsDuplicateKeys(t *testing.T) {
	root, _ := limitedFixture(t)
	if e := modelstore.WritePrivate(filepath.Join(root, "codex-limited-control.json"), []byte(`{"enabled":false,"enabled":true}`)); e != nil {
		t.Fatal(e)
	}
	var c LimitedControl
	if limitedRead(root, "codex-limited-control.json", &c) == nil {
		t.Fatal("duplicate control accepted")
	}
}
