//go:build windows

package agentartifact

import api "github.com/Garden12138/xcli/internal/agentapi"

// Reparse-point/handle parity is not accepted on Windows; fail closed.
func ReadBound(root, relative string, maxBytes int64) ([]byte, error) {
	return nil, api.Failure(503, "artifact_access_blocked")
}
