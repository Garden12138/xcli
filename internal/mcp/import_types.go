package mcp

import (
	"sort"

	"github.com/Garden12138/xcli/internal/config"
)

const (
	ImportActionAdd         = "add"
	ImportActionUpdate      = "update"
	ImportActionClaim       = "claim"
	ImportActionNoop        = "noop"
	ImportActionConflict    = "conflict"
	ImportActionUnsupported = "unsupported"
)

type ImportChange struct {
	Server  string   `json:"server"`
	Targets []string `json:"targets"`
	Action  string   `json:"action"`
	Status  string   `json:"status"`
	Detail  string   `json:"detail,omitempty"`

	desired   *config.MCPServer
	ownership map[string]Entry
}

type ImportPlan struct {
	SourceConfig string         `json:"source_config"`
	Scope        string         `json:"scope"`
	ProjectDir   string         `json:"project_dir,omitempty"`
	Targets      []string       `json:"targets"`
	Applicable   bool           `json:"applicable"`
	Applied      bool           `json:"applied"`
	Changes      []ImportChange `json:"changes"`

	verifySource      bool
	sourceExists      bool
	sourceFingerprint string
	nativeFiles       map[string]string
}

func (p *ImportPlan) Sort() {
	for index := range p.Changes {
		sort.Strings(p.Changes[index].Targets)
	}
	sort.Slice(p.Changes, func(i, j int) bool {
		if p.Changes[i].Server == p.Changes[j].Server {
			left := ""
			right := ""
			if len(p.Changes[i].Targets) > 0 {
				left = p.Changes[i].Targets[0]
			}
			if len(p.Changes[j].Targets) > 0 {
				right = p.Changes[j].Targets[0]
			}
			return left < right
		}
		return p.Changes[i].Server < p.Changes[j].Server
	})
}

func (p ImportPlan) HasChanges() bool {
	for _, change := range p.Changes {
		if change.Action == ImportActionAdd || change.Action == ImportActionUpdate || change.Action == ImportActionClaim {
			return true
		}
	}
	return false
}

type NativeCandidate struct {
	Name        string
	Target      string
	Server      config.MCPServer
	Fingerprint Entry
	Unsupported string
	Wrapper     *WrapperReference
}

type WrapperReference struct {
	SourceConfig string
	Server       string
}

type NativeSnapshot struct {
	Target      string
	Path        string
	Exists      bool
	Fingerprint string
	Entries     map[string]NativeCandidate
}
