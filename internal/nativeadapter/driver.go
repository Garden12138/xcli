// Package nativeadapter maps one authorized native turn to typed observations.
package nativeadapter

import (
	"context"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
)

type BoundRequest struct {
	TaskID        string
	SessionID     string
	NativeID      string
	Operation     string
	Prompt        string
	Workspace     string
	OutputRoot    string
	ReferencePath string
	Profile       agentpolicy.Profile
}
type Observation struct {
	Category  string
	Kind      string
	SessionID string
}
type Candidate struct {
	RelativePath   string
	Tool           *string
	EvidenceSHA256 *string
}
type TurnResult struct {
	Text                    string
	NativeID                string
	SessionObserved         bool
	ExitCode                *int
	Terminal                string
	Protocol                string
	LocalStopConfirmed      bool
	UpstreamCancelConfirmed *bool
	Code                    string
	Usage                   api.Usage
	Candidates              []Candidate
}

// Context cancellation is delivered only to this task's live worker controller.
// No public or persisted PID can be supplied to cancellation.
type Driver interface {
	StartTurn(context.Context, BoundRequest, func(Observation) error) TurnResult
}
