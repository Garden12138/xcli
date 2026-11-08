package agentpolicy

import (
	"errors"
	"strings"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
)

const EngineeringBlocked = "ENGINEERING_BLOCKED"
const EngineeringReady = "ENGINEERING_READY"
const UserConfirmationRequired = "USER_CONFIRMATION_REQUIRED"
const ReadyForAuthorization = "READY_FOR_AUTHORIZATION"

type LimitedConfirmation struct {
	Version                        int       `json:"version"`
	RegistrationSHA256             string    `json:"registration_sha256"`
	PolicySHA256                   string    `json:"policy_sha256"`
	ClientID                       string    `json:"client_id"`
	Reference                      string    `json:"reference"`
	OwnAuthorizedAccount           bool      `json:"own_authorized_account"`
	PlanAndCapabilityAvailable     bool      `json:"plan_and_capability_available"`
	PersonalLocalDevelopmentOnly   bool      `json:"personal_local_development_only"`
	NoSharingCommercialOrHosting   bool      `json:"no_sharing_commercial_or_hosting"`
	AllowSyntheticCloudRequest     bool      `json:"allow_synthetic_cloud_request"`
	UnderstandInternalCallsAndCost bool      `json:"understand_internal_calls_and_cost"`
	RecordedAt                     time.Time `json:"recorded_at"`
}

// Independent axes: engineering may be READY while user declarations remain
// missing. Neither a confirmation nor a readiness query authorizes a turn.
func resolveLimitedState(engineering, confirmed bool) (string, string, string) {
	eng, user := EngineeringBlocked, UserConfirmationRequired
	if engineering {
		eng = EngineeringReady
	}
	if confirmed {
		user = "CONFIRMED"
	}
	state := EngineeringBlocked
	if engineering {
		state = UserConfirmationRequired
		if confirmed {
			state = ReadyForAuthorization
		}
	}
	return state, eng, user
}
func confirmationValid(c LimitedConfirmation, r LimitedRegistration, cfg LimitedConfig) bool {
	return c.Version == 1 && c.RegistrationSHA256 == api.Digest(r) && c.PolicySHA256 == api.Digest(LimitedExecArgs(r.Workspace)) && c.ClientID == cfg.ClientID && validConfirmationReference(c.Reference) && c.OwnAuthorizedAccount && c.PlanAndCapabilityAvailable && c.PersonalLocalDevelopmentOnly && c.NoSharingCommercialOrHosting && c.AllowSyntheticCloudRequest && c.UnderstandInternalCallsAndCost && !c.RecordedAt.IsZero() && time.Since(c.RecordedAt) >= 0 && time.Since(c.RecordedAt) < 24*time.Hour
}
func validConfirmationReference(ref string) bool {
	return len(ref) >= 8 && len(ref) <= 128 && !strings.ContainsAny(ref, "@\r\n\x00")
}
func LimitedConfirmationTemplate(root string, cfg LimitedConfig) (LimitedConfirmation, error) {
	r, e := ReadLimited(root)
	if e != nil {
		return LimitedConfirmation{}, e
	}
	return LimitedConfirmation{Version: 1, RegistrationSHA256: api.Digest(r), PolicySHA256: api.Digest(LimitedExecArgs(r.Workspace)), ClientID: cfg.ClientID}, nil
}

// Owner-only local import. The CLI never infers these statements from login,
// a plan catalog, YAML, a previous authorization or a successful fake test.
func ConfirmLimited(root string, cfg LimitedConfig, c LimitedConfirmation) error {
	lock, e := LockLimited(root)
	if e != nil {
		return e
	}
	defer lock.Close()
	r, e := ReadLimited(root)
	if e != nil {
		return e
	}
	c.RecordedAt = time.Now().UTC()
	if !confirmationValid(c, r, cfg) {
		return errors.New("complete_current_owner_confirmation_required")
	}
	return limitedWrite(root, "codex-limited-confirmation.json", c)
}
func ConfirmLimitedJSON(root string, cfg LimitedConfig, b []byte) error {
	var c LimitedConfirmation
	if strictLimitedDecode(b, &c) != nil {
		return errors.New("invalid_owner_confirmation")
	}
	return ConfirmLimited(root, cfg, c)
}
func userConfirmation(root string, r LimitedRegistration, cfg LimitedConfig) (bool, string) {
	var c LimitedConfirmation
	if limitedRead(root, "codex-limited-confirmation.json", &c) != nil || !confirmationValid(c, r, cfg) {
		return false, ""
	}
	return true, api.Digest(c)
}

func boundaryProbeValid(p LimitedBoundaryProbe, r LimitedRegistration) bool {
	return p.Version == 1 && p.RegistrationSHA256 == api.Digest(r) && p.PolicySHA256 == api.Digest(LimitedExecArgs(r.Workspace)) && p.BinarySHA256 == r.BinarySHA256 && !p.RealCredentialsUsed && p.ModelRequests == 0 && time.Since(p.At) >= 0 && time.Since(p.At) < 15*time.Minute
}

// These are scoped evidence requirements, not an owner-writable "safe" flag.
// The current exec adapter provides no pre-execution builtin IO broker or
// version-bound effective tool-catalog attestation. File claims cannot add one.
func execBoundaryMissing() []string {
	return []string{"exec_effective_tool_catalog_not_attested", "direct_builtin_io_has_no_independent_credential_boundary", "exec_tool_environment_filter_not_runtime_verified"}
}
func nativeBoundaryVerified(root string, r LimitedRegistration) bool {
	var p LimitedBoundaryProbe
	if limitedRead(root, "codex-limited-boundary.json", &p) != nil || !boundaryProbeValid(p, r) {
		return false
	}
	for _, name := range []string{"selected_read", "fake_auth_absolute_read_denied", "fake_auth_relative_read_denied", "outside_read_denied", "symlink_fake_auth_read_denied", "workspace_write_denied", "mock_service_tool_network_denied", "workspace_unchanged", "binary_unchanged"} {
		if p.Observations[name].Result != "PASS" {
			return false
		}
	}
	return len(execBoundaryMissing()) == 0
}
