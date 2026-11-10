// Package siwc contains side-effect-free G8 protocol contracts. It has no live
// OAuth client, HTTP transport, process launcher, or production provider factory.
package siwc

import (
	"errors"
	"fmt"
)

const (
	ProviderType      = "siwc-responses"
	ProductionStatus  = "BLOCKED_ELIGIBILITY_AND_ARCHITECTURE"
	Issuer            = "https://auth.openai.com"
	Resource          = "https://api.openai.com/v1"
	AuthorizeEndpoint = Issuer + "/api/accounts/authorize"
	TokenEndpoint     = Issuer + "/api/accounts/oauth/token"
	DynamicClient     = "dynamic_agent_client"
	Scopes            = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
)

var ErrSecureStoreUnavailable = errors.New("siwc_secure_store_unavailable: OS credential adapter is not implemented; no fallback")

// Secret prevents accidental formatting/JSON disclosure. Explicit access remains
// private to this package. This is log hygiene, not memory or OS isolation.
type Secret struct{ value string }

func (Secret) Format(s fmt.State, _ rune)   { _, _ = s.Write([]byte("[REDACTED]")) }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

// SecureStore must atomically replace the whole record and serialize refreshes
// across processes. Production implementations must use OS credential services.
// No filesystem or in-memory implementation is provided in production.
type SecureStore interface {
	Load(registration string) (Credential, error)
	Replace(registration string, expectedGeneration uint64, next Credential) error
	Clear(registration string) error
}

func OpenSystemStore() (SecureStore, error) { return nil, ErrSecureStoreUnavailable }
