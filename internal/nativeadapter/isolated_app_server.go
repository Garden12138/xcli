package nativeadapter

import (
	"context"

	"github.com/Garden12138/xcli/internal/agent"
	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/isolatedruntime"
)

// IsolatedAppServer is an offline feasibility transport, default disabled.
// Production serve never selects it and native Profile.Check still fails.
// Actual integration turns in G7 use an explicitly fake Linux peer only.
type IsolatedAppServer struct {
	Enabled bool
	Runtime *isolatedruntime.Docker
}

func (d IsolatedAppServer) StartTurn(ctx context.Context, b BoundRequest, observe func(Observation) error) TurnResult {
	closed := TurnResult{Code: "policy_unenforceable", Protocol: "not_started", Terminal: "missing", LocalStopConfirmed: true, Usage: api.Usage{Source: "none", Scope: "unknown"}}
	if !d.Enabled || d.Runtime == nil {
		return closed
	}
	if e := b.Profile.Check(b.Operation); e != nil {
		return closed
	}
	if !d.Runtime.Acquire() {
		closed.Code = "busy"
		return closed
	}
	defer d.Runtime.Release()
	b.Workspace = "/workspace"
	adapter := AppServer{launch: func(bound BoundRequest, _ string) (agent.CommandSpec, string, []string, error) {
		return d.Runtime.Spec(bound.TaskID, AppServerArgs())
	}}
	result := adapter.StartTurn(ctx, b, observe)
	if !d.Runtime.StopConfirmed(b.TaskID) {
		result.LocalStopConfirmed = false
		result.Code = "execution_state_unknown"
	}
	return result
}
