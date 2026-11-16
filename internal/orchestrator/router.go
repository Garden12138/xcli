package orchestrator

import (
	"fmt"
	"slices"
	"sort"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/proc"
)

type Router struct{ env *Env }

func NewRouter(e *Env) *Router { return &Router{env: e} }

func (r *Router) eligible() []string {
	e := r.env
	allowed := e.Req.Agents
	if len(allowed) == 0 {
		allowed = config.AgentOrder
	}
	if e.Req.Agent != "" {
		allowed = []string{e.Req.Agent}
	}
	choices := []string{}
	for _, name := range allowed {
		d := e.Drivers[name]
		if d == nil || !e.Config.Enabled(name) || slices.Contains(choices, name) {
			continue
		}
		if e.Health != nil && !e.Health.Available(name, e.now()) {
			continue
		}
		if _, err := proc.LookPath(d.Command(), e.PathList); err != nil {
			continue
		}
		choices = append(choices, name)
	}
	return choices
}

func (r *Router) Candidates(kind api.Kind, exclude ...string) []string {
	e := r.env
	choices := slices.DeleteFunc(r.eligible(), func(name string) bool {
		return slices.Contains(exclude, name) || e.Config.Strength(name, string(kind)) <= 0
	})
	sort.SliceStable(choices, func(i, j int) bool {
		return e.Config.Strength(choices[i], string(kind)) > e.Config.Strength(choices[j], string(kind))
	})
	return choices
}

func (r *Router) Pick(kind api.Kind, exclude ...string) (string, error) {
	choices := r.Candidates(kind, exclude...)
	if len(choices) == 0 {
		return "", fmt.Errorf("请运行 xcli agents 检查安装和冷却状态，并为 %s 启用可用 CLI", kind)
	}
	return choices[0], nil
}
