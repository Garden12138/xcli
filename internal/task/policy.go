package task

// These are native invocation policies, not an xcli filesystem sandbox. Their
// availability and enforcement are verified per CLI/version in native smoke.
// No role enables force/yolo, skips permissions, or edits persistent rules.
func roleArgs(adapter, role string) []string {
	readOnly := role == "coordinator" || role == "reviewer"
	switch adapter {
	case "codex":
		mode := "workspace-write"
		if readOnly {
			mode = "read-only"
		}
		return []string{"--skip-git-repo-check", "--sandbox", mode}
	case "cursor":
		if readOnly {
			return []string{"--mode", "ask"}
		}
		return []string{"--sandbox", "enabled"}
	case "antigravity":
		mode := "accept-edits"
		if readOnly {
			mode = "plan"
		}
		return []string{"--mode", mode}
	}
	return nil
}
