package nativeadapter

import (
	"errors"

	"github.com/Garden12138/xcli/internal/agentpolicy"
)

// Preflight is owner control-plane evidence, never a capability promotion or an
// HTTP API. It reads only the selected installation file; no process is started.
type Preflight struct {
	Version       int              `json:"version"`
	AgentID       string           `json:"agent_id"`
	AdapterID     string           `json:"adapter_id"`
	ProfileSHA256 string           `json:"profile_sha256"`
	CanExecute    bool             `json:"can_execute"`
	ProcessStarts int              `json:"process_starts"`
	ModelRequests int              `json:"model_requests"`
	Checks        []PreflightCheck `json:"checks"`
}
type PreflightCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

func InspectNativeText(p agentpolicy.Profile, cfg agentpolicy.Config) (Preflight, error) {
	r := Preflight{Version: 1, AgentID: p.AgentID, AdapterID: p.AdapterID, ProfileSHA256: p.SHA256, Checks: []PreflightCheck{}}
	registered := false
	for _, d := range agentpolicy.Defaults() {
		registered = registered || (p.Native && p.AgentID == d.AgentID && p.AdapterID == d.AdapterID)
	}
	if !registered {
		return r, errors.New("registered native profile required")
	}
	if err := cfg.Validate(); err != nil {
		return r, err
	}
	add := func(id, status, evidence string) { r.Checks = append(r.Checks, PreflightCheck{id, status, evidence}) }
	add("configuration", "PASS", "validated_independent_config")
	if cfg.DailyDispatches == 0 || cfg.DailyProcessStarts == 0 || cfg.DailyExecutionSeconds == 0 {
		add("global_budget", "BLOCKED", "zero_global_budget")
	} else {
		add("global_budget", "PASS", "configured_ceiling_only_not_call_authorization")
	}
	if p.Version == nil || p.Binary == "" || p.BinarySHA256 == "" {
		add("installation_cache", "NOT_TESTED", "owner_refresh_required")
	} else {
		add("installation_cache", "PASS", "cached_version_only_not_protocol_or_entitlement")
	}
	if err := p.CheckBinary(); err != nil {
		add("selected_file_integrity", "BLOCKED", err.Error())
	} else {
		add("selected_file_integrity", "PASS", "selected_file_hash_only_runtime_dependencies_unverified")
	}
	// Keep the actual execution gate authoritative. Clearing Blocker, refreshing
	// a version or increasing budgets must not manufacture an enabled profile.
	if err := p.Check("text_task"); err != nil {
		add("execution_policy", "BLOCKED", err.Error())
	} else {
		return r, errors.New("native promotion requires a new reviewed preflight implementation")
	}
	add("native_auth_environment", "BLOCKED", "synthetic_home_has_no_verified_native_auth_binding")
	add("filesystem_tools_network", "BLOCKED", "per_invocation_enforcement_not_implemented")
	add("account_entitlement", "NOT_TESTED", "no_native_account_or_model_entitlement_evidence")
	add("application_authorization", "NOT_TESTED", "provider_terms_and_owner_use_case_review_required")
	add("native_approval", "PASS", "offline_deny_only_contract_no_http_approve_route")
	if p.AgentID == "cursor" {
		add("workspace_trust", "BLOCKED", "historical_denial_requires_owner_review_of_exact_workspace")
	}
	add("protocol_transport", "NOT_TESTED", "installed_handshake_does_not_verify_turn_semantics")
	add("real_http_text", "NOT_TESTED", "new_explicit_inference_authorization_required")
	return r, nil
}
