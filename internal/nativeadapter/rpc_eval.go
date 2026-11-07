package nativeadapter

import (
	"encoding/json"
	"errors"
)

// RPCEvaluation is a static, deny-only protocol assessment, not an enabled
// app-server transport. No subprocess, credential or network access is present.
type RPCEvaluation struct {
	Initialized bool
	Pending     map[int]string
	Interrupted bool
}

func (r *RPCEvaluation) Request(id int, method string) error {
	if r.Pending == nil {
		r.Pending = map[int]string{}
	}
	if _, exists := r.Pending[id]; exists {
		return errors.New("duplicate request id")
	}
	if method != "initialize" && !r.Initialized {
		return errors.New("not initialized")
	}
	switch method {
	case "initialize":
		if r.Initialized {
			return errors.New("already initialized")
		}
	case "thread/start", "thread/resume", "turn/start", "turn/interrupt":
	default:
		return errors.New("method not permitted")
	}
	r.Pending[id] = method
	return nil
}
func (r *RPCEvaluation) Reply(id int) error {
	method, ok := r.Pending[id]
	if !ok {
		return errors.New("unmatched response")
	}
	delete(r.Pending, id)
	if method == "initialize" {
		r.Initialized = true
	}
	return nil
}
func (r *RPCEvaluation) ServerRequest(id int, method string) []byte {
	var v any
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		v = map[string]any{"id": id, "result": map[string]any{"decision": "decline"}}
	case "item/permissions/requestApproval":
		v = map[string]any{"id": id, "result": map[string]any{"permissions": map[string]any{}}}
	default:
		v = map[string]any{"id": id, "error": map[string]any{"code": -32601, "message": "method not permitted"}}
	}
	b, _ := json.Marshal(v)
	return b
}
func (r *RPCEvaluation) Notification(method, status string) {
	if method == "turn/completed" && status == "interrupted" {
		r.Interrupted = true
	}
}
