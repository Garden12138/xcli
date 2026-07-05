package mcp

import (
	"context"
	"fmt"
	"sort"

	"github.com/Garden12138/xcli/internal/config"
)

type BuildOptions struct {
	SourceConfig  string
	Launcher      string
	Scope         string
	ProjectDir    string
	ProjectConfig string
	Targets       []string
	Unavailable   []string
	Force         bool
}

func BuildPlan(ctx context.Context, cfg config.Config, managers map[string]Manager, state *State, options BuildOptions) (Plan, error) {
	if options.Scope == "" {
		options.Scope = ScopeUser
	}
	if options.Scope != ScopeUser && options.Scope != ScopeProject {
		return Plan{}, fmt.Errorf("unsupported MCP scope %q", options.Scope)
	}
	if options.Scope == ScopeProject && (options.ProjectDir == "" || options.ProjectConfig == "") {
		return Plan{}, fmt.Errorf("project MCP scope requires a project directory and relative source configuration")
	}
	plan := Plan{
		SourceConfig: options.SourceConfig,
		Launcher:     options.Launcher,
		Scope:        options.Scope,
		ProjectDir:   options.ProjectDir,
		Targets:      append(append([]string(nil), options.Targets...), options.Unavailable...),
		Applicable:   true,
		Changes:      []Change{},
	}
	sort.Strings(plan.Targets)
	for _, target := range options.Unavailable {
		plan.Changes = append(plan.Changes, Change{
			Target: target, Action: ActionUnavailable, Status: StatusSkipped,
			Detail: "target CLI is not installed",
		})
		plan.Applicable = false
	}
	for _, target := range options.Targets {
		manager := managers[target]
		if manager == nil {
			return Plan{}, fmt.Errorf("unsupported MCP target %q", target)
		}
		if preflight, ok := manager.(interface{ Preflight(context.Context) error }); ok {
			if err := preflight.Preflight(ctx); err != nil {
				return Plan{}, fmt.Errorf("check %s MCP command: %w", target, err)
			}
		}
		current, err := manager.Load(ctx)
		if err != nil {
			return Plan{}, fmt.Errorf("read %s MCP configuration: %w", target, err)
		}
		desired := desiredEntries(cfg, options, target)
		names := map[string]bool{}
		for name := range desired {
			names[name] = true
		}
		namespace := NamespaceKey(options.Scope, options.ProjectDir)
		for name, owner := range state.EntriesFor(namespace)[target] {
			if owner.SourceConfig == options.SourceConfig {
				names[name] = true
			}
		}
		ordered := make([]string, 0, len(names))
		for name := range names {
			ordered = append(ordered, name)
		}
		sort.Strings(ordered)
		for _, name := range ordered {
			change := planChange(target, name, options.SourceConfig, namespace, current, desired, state, options.Force)
			if change.Action == ActionConflict {
				plan.Applicable = false
			}
			plan.Changes = append(plan.Changes, change)
		}
	}
	plan.Sort()
	return plan, nil
}

func desiredEntries(cfg config.Config, options BuildOptions, target string) map[string]Entry {
	result := map[string]Entry{}
	for name, server := range cfg.MCP.Servers {
		if !serverTargets(server, target) {
			continue
		}
		if server.Transport == "http" {
			result[name] = Entry{Transport: "http", URL: server.URL}
			continue
		}
		result[name] = Entry{
			Transport: "stdio", Command: options.Launcher,
			EnvVars: append([]string(nil), server.EnvVars...),
		}
		if options.Scope == ScopeProject {
			entry := result[name]
			entry.Args = []string{"mcp", "serve", "--project-config", options.ProjectConfig, name}
			result[name] = entry
		} else {
			entry := result[name]
			entry.Args = []string{"--config", options.SourceConfig, "mcp", "serve", name}
			result[name] = entry
		}
	}
	return result
}

func serverTargets(server config.MCPServer, target string) bool {
	if len(server.Targets) == 0 {
		return true
	}
	for _, value := range server.Targets {
		if value == target {
			return true
		}
	}
	return false
}

func planChange(target, name, source, namespace string, current, desired map[string]Entry, state *State, force bool) Change {
	currentEntry, hasCurrent := current[name]
	desiredEntry, hasDesired := desired[name]
	owner, owned := state.GetIn(namespace, target, name)
	change := Change{Target: target, Server: name, Status: StatusPlanned}
	if hasCurrent {
		change.current = entryPointer(currentEntry)
	}
	if hasDesired {
		change.desired = entryPointer(desiredEntry)
	}

	conflict := func(detail string) Change {
		if force {
			if hasDesired {
				if hasCurrent {
					change.Action = ActionUpdate
				} else {
					change.Action = ActionAdd
				}
			} else {
				change.Action = ActionRemove
			}
			change.Detail = "forced: " + detail
			return change
		}
		change.Action = ActionConflict
		change.Status = StatusSkipped
		change.Detail = detail
		return change
	}

	if hasDesired {
		if !owned {
			if hasCurrent {
				return conflict("same-name native entry is not managed by xcli")
			}
			change.Action = ActionAdd
			return change
		}
		if owner.SourceConfig != source {
			return conflict("entry is managed by another xcli configuration")
		}
		if !hasCurrent {
			return conflict("managed native entry was removed outside xcli")
		}
		if currentEntry.Fingerprint() != owner.Fingerprint {
			return conflict("managed native entry changed outside xcli")
		}
		if currentEntry.Fingerprint() == desiredEntry.Fingerprint() {
			change.Action = ActionNoop
			change.Status = StatusSkipped
			return change
		}
		change.Action = ActionUpdate
		return change
	}

	if !owned || owner.SourceConfig != source {
		change.Action = ActionNoop
		change.Status = StatusSkipped
		return change
	}
	if !hasCurrent {
		return conflict("managed native entry was removed outside xcli")
	}
	if currentEntry.Fingerprint() != owner.Fingerprint {
		return conflict("managed native entry changed outside xcli")
	}
	change.Action = ActionRemove
	return change
}

func entryPointer(value Entry) *Entry {
	copy := value
	copy.Args = append([]string(nil), value.Args...)
	copy.EnvVars = append([]string(nil), value.EnvVars...)
	return &copy
}

func ApplyPlan(ctx context.Context, plan *Plan, managers map[string]Manager, state *State) error {
	if !plan.Applicable {
		return fmt.Errorf("MCP sync plan contains conflicts or unavailable targets")
	}
	if err := state.Save(); err != nil {
		return err
	}
	namespace := NamespaceKey(plan.Scope, plan.ProjectDir)
	for index := range plan.Changes {
		change := &plan.Changes[index]
		if change.Action == ActionNoop || change.Action == ActionUnavailable || change.Action == ActionConflict {
			continue
		}
		manager := managers[change.Target]
		if !(change.Action == ActionRemove && change.current == nil) {
			if err := manager.Apply(ctx, *change); err != nil {
				change.Status = StatusFailed
				change.Detail = err.Error()
				return fmt.Errorf("apply %s MCP change for %s/%s: %w", change.Action, change.Target, change.Server, err)
			}
		}
		if change.Action == ActionRemove {
			state.DeleteIn(namespace, change.Target, change.Server)
		} else {
			state.SetIn(namespace, change.Target, change.Server, Ownership{
				SourceConfig: plan.SourceConfig,
				Fingerprint:  change.desired.Fingerprint(),
			})
		}
		if err := state.Save(); err != nil {
			change.Status = StatusFailed
			change.Detail = err.Error()
			return err
		}
		change.Status = StatusApplied
	}
	plan.Applied = true
	return nil
}
