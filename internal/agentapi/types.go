// Package agentapi defines the independent Native Agent Gateway V1 wire contract.
package agentapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

const Prefix = "/agent/v1"
const MaxJSON = 1 << 20
const MaxText = 1 << 20

func ID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("secure randomness unavailable")
	}
	return prefix + "_" + hex.EncodeToString(b)
}
func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Digest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return Hash(b)
}

type Error struct {
	Status                int     `json:"-"`
	Code                  string  `json:"code"`
	Category              string  `json:"category"`
	Message               string  `json:"message"`
	Accepted              bool    `json:"accepted"`
	TaskID                *string `json:"task_id"`
	AutomaticRetryAllowed bool    `json:"automatic_retry_allowed"`
	Remediation           string  `json:"remediation"`
}

func (e *Error) Error() string { return e.Code }
func Failure(status int, code string) *Error {
	category := "validation"
	remediation := "none"
	switch status {
	case 401:
		category = "auth"
	case 403:
		category = "permission"
	case 409, 410:
		category = "state"
	case 429:
		category = "budget"
	case 503:
		category = "availability"
	}
	switch code {
	case "capability_missing", "capability_unverified":
		category = "capability"
		remediation = "owner_review_required"
	case "budget_exhausted", "budget_unenforceable":
		category = "budget"
	case "protocol_error", "native_session_lost", "output_limit":
		category = "protocol"
	case "timeout", "queue_timeout", "canceled", "start_failed", "native_process_failed":
		category = "execution"
	case "execution_state_unknown":
		category = "execution"
		remediation = "reconcile_required"
	case "invalid_image", "input_hash_mismatch", "provenance_unverified", "validation_failed", "artifact_not_ready", "artifact_access_blocked", "review_incomplete":
		category = "artifact"
	}
	switch code {
	case "invalid_client_token", "native_auth_required":
		category = "auth"
	case "scope_denied", "cloud_consent_required", "policy_denied", "origin_denied", "workspace_untrusted", "approval_required":
		category = "permission"
	case "agent_not_installed", "adapter_unavailable", "policy_unenforceable", "service_stopping", "service_restarted", "native_tool_unavailable":
		category = "availability"
	case "native_quota_exceeded", "extra_payment_required":
		category = "budget"
	case "not_found", "agent_not_found", "idempotency_conflict", "session_busy", "session_revision_changed", "capability_changed", "consent_scope_changed", "profile_changed", "object_expired", "event_cursor_expired", "idempotent_result_expired":
		category = "state"
	case "store_unavailable", "store_corrupt":
		category = "internal"
	}
	switch code {
	case "workspace_untrusted", "native_auth_required", "native_quota_exceeded", "extra_payment_required", "approval_required", "policy_unenforceable", "profile_changed":
		remediation = "owner_review_required"
	case "native_session_lost":
		remediation = "reconcile_required"
	}
	// Neither arbitrary native diagnostics nor user content is reflected in errors.
	return &Error{Status: status, Code: code, Category: category, Message: "Native Agent Gateway: " + code, Remediation: remediation}
}

type Budget struct {
	MaxDispatches       int             `json:"max_dispatches"`
	MaxProcessStarts    int             `json:"max_process_starts"`
	MaxExecutionSeconds int             `json:"max_execution_seconds,omitempty"`
	TimeoutSeconds      int             `json:"timeout_seconds,omitempty"`
	MaxCostUSD          json.RawMessage `json:"max_cost_usd,omitempty"`
}
type BudgetState struct {
	MaxDispatches            int     `json:"max_dispatches"`
	MaxProcessStarts         int     `json:"max_process_starts"`
	MaxExecutionSeconds      int     `json:"max_execution_seconds"`
	DispatchesConsumed       int     `json:"dispatches_consumed"`
	ProcessStartsConsumed    int     `json:"process_starts_consumed"`
	ExecutionSeconds         float64 `json:"execution_seconds"`
	ExecutionSecondsReserved int     `json:"execution_seconds_reserved"`
}
type Context struct {
	App       string `json:"app"`
	Workflow  string `json:"workflow"`
	OpaqueRef string `json:"opaque_ref"`
}
type SessionCreate struct {
	AgentID            string  `json:"agent_id"`
	AdapterID          string  `json:"adapter_id"`
	CapabilityRevision string  `json:"capability_revision"`
	PolicyID           string  `json:"policy_id"`
	Budget             Budget  `json:"budget"`
	Context            Context `json:"context"`
}
type Session struct {
	APIVersion         int         `json:"api_version"`
	ID                 string      `json:"id"`
	Revision           int         `json:"revision"`
	State              string      `json:"state"`
	BindingState       string      `json:"binding_state"`
	AgentID            string      `json:"agent_id"`
	AdapterID          string      `json:"adapter_id"`
	CapabilityRevision string      `json:"capability_revision"`
	PolicyID           string      `json:"policy_id"`
	PolicyRevision     string      `json:"policy_revision"`
	ProfileSHA256      string      `json:"profile_sha256"`
	ContextSHA256      string      `json:"context_sha256"`
	Budget             BudgetState `json:"budget"`
	ActiveTaskID       *string     `json:"active_task_id"`
	CreatedAt          time.Time   `json:"created_at"`
	ExpiresAt          time.Time   `json:"expires_at"`
	InferenceLocation  string      `json:"inference_location"`
	NativeModel        *string     `json:"native_model"`
}
type Part struct {
	Type    string  `json:"type"`
	Text    *string `json:"text,omitempty"`
	InputID *string `json:"input_id,omitempty"`
}
type Consent struct {
	AllowRemote   bool   `json:"allow_remote"`
	ContextSHA256 string `json:"context_sha256"`
}
type TaskCreate struct {
	SessionID               string  `json:"session_id"`
	ExpectedSessionRevision int     `json:"expected_session_revision"`
	Operation               string  `json:"operation"`
	Input                   []Part  `json:"input"`
	Budget                  Budget  `json:"budget"`
	Consent                 Consent `json:"consent"`
}
type Execution struct {
	State                   string `json:"state"`
	NativeExitCode          *int   `json:"native_exit_code"`
	NativeTerminal          string `json:"native_terminal"`
	LocalStopConfirmed      *bool  `json:"local_stop_confirmed"`
	UpstreamCancelConfirmed *bool  `json:"upstream_cancel_confirmed"`
	SideEffects             string `json:"side_effects"`
}
type Protocol struct {
	Status                string `json:"status"`
	NativeSessionObserved bool   `json:"native_session_observed"`
}
type Validation struct {
	Status     string `json:"status"`
	Bytes      string `json:"bytes"`
	Provenance string `json:"provenance"`
	Semantics  string `json:"semantics"`
}
type Result struct {
	Text        *string  `json:"text"`
	Final       bool     `json:"final"`
	Artifacts   []string `json:"artifacts"`
	NativeModel *string  `json:"native_model"`
}
type Usage struct {
	Source          string   `json:"source"`
	Scope           string   `json:"scope"`
	InputTokens     *int64   `json:"input_tokens"`
	OutputTokens    *int64   `json:"output_tokens"`
	TotalTokens     *int64   `json:"total_tokens"`
	CacheReadTokens *int64   `json:"cache_read_tokens"`
	ReasoningTokens *int64   `json:"reasoning_tokens"`
	CostUSD         *float64 `json:"cost_usd"`
	RemainingQuota  *float64 `json:"remaining_quota"`
}
type TaskBudget struct {
	DispatchesConsumed    int     `json:"dispatches_consumed"`
	ProcessStartsConsumed int     `json:"process_starts_consumed"`
	ExecutionSeconds      float64 `json:"execution_seconds"`
}
type Task struct {
	APIVersion         int        `json:"api_version"`
	ID                 string     `json:"id"`
	SessionID          string     `json:"session_id"`
	AgentID            string     `json:"agent_id"`
	AdapterID          string     `json:"adapter_id"`
	ProfileSHA256      string     `json:"profile_sha256"`
	CapabilityRevision string     `json:"capability_revision"`
	Operation          string     `json:"operation"`
	Status             string     `json:"status"`
	Revision           int        `json:"revision"`
	Execution          Execution  `json:"execution"`
	Protocol           Protocol   `json:"protocol"`
	Validation         Validation `json:"validation"`
	Result             Result     `json:"result"`
	Usage              Usage      `json:"usage"`
	Budget             TaskBudget `json:"budget"`
	Error              *Error     `json:"error"`
	CreatedAt          time.Time  `json:"created_at"`
	DeadlineAt         time.Time  `json:"deadline_at"`
	EndedAt            *time.Time `json:"ended_at"`
	ExpiresAt          time.Time  `json:"expires_at"`
	LastEventSeq       int64      `json:"last_event_seq"`
	ReviewDeadlineAt   *time.Time `json:"review_deadline_at,omitempty"`
}

func Terminal(s string) bool {
	switch s {
	case "succeeded", "blocked", "failed", "canceled", "timed_out", "expired", "needs_reconcile":
		return true
	}
	return false
}

type Event struct {
	APIVersion int            `json:"api_version"`
	TaskID     string         `json:"task_id"`
	SessionID  string         `json:"session_id"`
	Seq        int64          `json:"seq"`
	Time       time.Time      `json:"time"`
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload"`
}
type Operation struct {
	Documented      bool       `json:"documented"`
	NativeEvidence  string     `json:"native_evidence"`
	GatewayEvidence string     `json:"gateway_evidence"`
	Availability    string     `json:"availability"`
	Reason          string     `json:"reason"`
	EvidenceRef     *string    `json:"evidence_ref"`
	VerifiedAt      *time.Time `json:"verified_at"`
}
type Agent struct {
	AgentID           string               `json:"agent_id"`
	AdapterID         string               `json:"adapter_id"`
	InstalledVersion  *string              `json:"installed_version"`
	BinarySHA256      *string              `json:"binary_sha256"`
	InferenceLocation string               `json:"inference_location"`
	Authentication    string               `json:"authentication"`
	RemainingQuota    *float64             `json:"remaining_quota"`
	NativeModel       *string              `json:"native_model"`
	Operations        map[string]Operation `json:"operations"`
	Features          map[string]any       `json:"features"`
	Policies          []string             `json:"policies"`
}
type Capabilities struct {
	APIVersion  int       `json:"api_version"`
	InstanceID  string    `json:"instance_id"`
	Revision    string    `json:"revision"`
	GeneratedAt time.Time `json:"generated_at"`
	Agents      []Agent   `json:"agents"`
}
