package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

const (
	ScopeUser    = "user"
	ScopeProject = "project"

	ActionAdd         = "add"
	ActionUpdate      = "update"
	ActionRemove      = "remove"
	ActionNoop        = "noop"
	ActionConflict    = "conflict"
	ActionUnavailable = "unavailable"

	StatusPlanned = "planned"
	StatusApplied = "applied"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
)

var Targets = []string{"claude", "codex", "gemini", "opencode"}

type Entry struct {
	Transport string   `json:"transport"`
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	EnvVars   []string `json:"env_vars,omitempty"`
	URL       string   `json:"url,omitempty"`
}

func (e Entry) Fingerprint() string {
	e.EnvVars = append([]string(nil), e.EnvVars...)
	sort.Strings(e.EnvVars)
	data, _ := json.Marshal(e)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type Change struct {
	Target string `json:"target"`
	Server string `json:"server,omitempty"`
	Action string `json:"action"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`

	current *Entry
	desired *Entry
}

type Plan struct {
	SourceConfig string   `json:"source_config"`
	Launcher     string   `json:"launcher,omitempty"`
	Scope        string   `json:"scope"`
	ProjectDir   string   `json:"project_dir,omitempty"`
	Targets      []string `json:"targets"`
	Applicable   bool     `json:"applicable"`
	Applied      bool     `json:"applied"`
	Changes      []Change `json:"changes"`
}

func (p *Plan) Sort() {
	sort.Slice(p.Changes, func(i, j int) bool {
		if p.Changes[i].Target == p.Changes[j].Target {
			return p.Changes[i].Server < p.Changes[j].Server
		}
		return p.Changes[i].Target < p.Changes[j].Target
	})
}

func (p Plan) UsesLauncherChanges() bool {
	for _, change := range p.Changes {
		if change.Action != ActionAdd && change.Action != ActionUpdate && change.Action != ActionRemove {
			continue
		}
		if (change.desired != nil && change.desired.Transport == "stdio") ||
			(change.desired == nil && change.current != nil && change.current.Transport == "stdio") {
			return true
		}
	}
	return false
}

func IsTarget(value string) bool {
	for _, target := range Targets {
		if target == value {
			return true
		}
	}
	return false
}
