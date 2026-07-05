package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Garden12138/xcli/internal/config"
)

type ImportBuildOptions struct {
	SourceConfig      string
	Scope             string
	ProjectDir        string
	Targets           []string
	Force             bool
	VerifySource      bool
	SourceExists      bool
	SourceFingerprint string
}

func BuildImportPlan(cfg config.Config, snapshots map[string]NativeSnapshot, state *State, options ImportBuildOptions) (ImportPlan, error) {
	if options.Scope == "" {
		options.Scope = ScopeUser
	}
	if options.Scope != ScopeUser && options.Scope != ScopeProject {
		return ImportPlan{}, fmt.Errorf("unsupported MCP import scope %q", options.Scope)
	}
	plan := ImportPlan{
		SourceConfig:      options.SourceConfig,
		Scope:             options.Scope,
		ProjectDir:        options.ProjectDir,
		Targets:           append([]string(nil), options.Targets...),
		Applicable:        true,
		Changes:           []ImportChange{},
		verifySource:      options.VerifySource,
		sourceExists:      options.SourceExists,
		sourceFingerprint: options.SourceFingerprint,
		nativeFiles:       map[string]string{},
	}
	sort.Strings(plan.Targets)
	names := map[string]bool{}
	for _, target := range options.Targets {
		if snapshot := snapshots[target]; snapshot.Path != "" {
			plan.nativeFiles[snapshot.Path] = snapshot.Fingerprint
		}
		for name := range snapshots[target].Entries {
			names[name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	namespace := NamespaceKey(options.Scope, options.ProjectDir)
	canonicalSource := canonicalBestEffort(options.SourceConfig)

	for _, name := range ordered {
		candidates := []NativeCandidate{}
		for _, target := range options.Targets {
			if candidate, ok := snapshots[target].Entries[name]; ok {
				candidates = append(candidates, candidate)
			}
		}
		change := buildImportChange(name, candidates, cfg.MCP.Servers[name], cfg.MCP.Servers, state, namespace, canonicalSource, options.Force)
		if change.Action == ImportActionConflict {
			plan.Applicable = false
		}
		plan.Changes = append(plan.Changes, change)
	}
	plan.Sort()
	return plan, nil
}

func buildImportChange(name string, candidates []NativeCandidate, existing config.MCPServer, allExisting map[string]config.MCPServer, state *State, namespace, source string, force bool) ImportChange {
	targets := candidateTargets(candidates)
	change := ImportChange{Server: name, Targets: targets, Status: StatusPlanned, ownership: map[string]Entry{}}
	unsupported := []string{}
	resolved := make([]NativeCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Unsupported != "" {
			unsupported = append(unsupported, candidate.Target+": "+candidate.Unsupported)
			continue
		}
		if candidate.Wrapper != nil {
			declared, exists := allExisting[name]
			switch {
			case candidate.Wrapper.SourceConfig == "":
				unsupported = append(unsupported, candidate.Target+": generated xcli wrapper has no resolvable source configuration")
				continue
			case canonicalBestEffort(candidate.Wrapper.SourceConfig) != source:
				unsupported = append(unsupported, candidate.Target+": generated xcli wrapper belongs to another source configuration")
				continue
			case candidate.Wrapper.Server != name:
				unsupported = append(unsupported, candidate.Target+": generated xcli wrapper server name does not match the native entry")
				continue
			case !exists:
				unsupported = append(unsupported, candidate.Target+": generated xcli wrapper has no declaration in the source configuration")
				continue
			default:
				candidate.Server = cloneMCPServer(declared)
				candidate.Server.Targets = nil
			}
		}
		resolved = append(resolved, candidate)
	}
	if len(unsupported) > 0 {
		change.Action = ImportActionUnsupported
		change.Status = StatusSkipped
		change.Detail = strings.Join(unsupported, "; ")
		return change
	}
	if len(resolved) == 0 {
		change.Action = ImportActionUnsupported
		change.Status = StatusSkipped
		change.Detail = "no importable native definition"
		return change
	}
	candidateServer := cloneMCPServer(resolved[0].Server)
	candidateServer.Targets = nil
	for _, candidate := range resolved[1:] {
		value := cloneMCPServer(candidate.Server)
		value.Targets = nil
		if mcpServerFingerprint(value) != mcpServerFingerprint(candidateServer) {
			change.Action = ImportActionConflict
			change.Status = StatusSkipped
			change.Detail = "selected agents define this server differently"
			return change
		}
	}
	for _, candidate := range resolved {
		change.ownership[candidate.Target] = candidate.Fingerprint
	}

	hasExisting := existing.Transport != ""
	desired := cloneMCPServer(candidateServer)
	desired.Targets = append([]string(nil), targets...)
	configChanged := false
	forcedDefinition := false
	if hasExisting {
		existingCore := cloneMCPServer(existing)
		existingCore.Targets = nil
		if mcpServerFingerprint(existingCore) != mcpServerFingerprint(candidateServer) {
			if !force {
				change.Action = ImportActionConflict
				change.Status = StatusSkipped
				change.Detail = "xcli source configuration defines this server differently"
				return change
			}
			desired.Targets = mergedTargetCoverage(existing.Targets, targets)
			configChanged = true
			forcedDefinition = true
		} else {
			desired = cloneMCPServer(existing)
			merged := mergedTargetCoverage(existing.Targets, targets)
			if !sameStrings(existing.Targets, merged) {
				desired.Targets = merged
				configChanged = true
			}
		}
	} else {
		configChanged = true
	}

	ownershipChanged := false
	ownershipForced := false
	for target, fingerprint := range change.ownership {
		owner, owned := state.GetIn(namespace, target, name)
		if !owned {
			ownershipChanged = true
			continue
		}
		if canonicalBestEffort(owner.SourceConfig) != source || owner.Fingerprint != fingerprint.Fingerprint() {
			if !force {
				change.Action = ImportActionConflict
				change.Status = StatusSkipped
				if canonicalBestEffort(owner.SourceConfig) != source {
					change.Detail = "native entry is owned by another xcli source configuration"
				} else {
					change.Detail = "managed native entry changed outside xcli"
				}
				return change
			}
			ownershipChanged = true
			ownershipForced = true
		}
	}

	change.desired = &desired
	switch {
	case !hasExisting:
		change.Action = ImportActionAdd
	case configChanged:
		change.Action = ImportActionUpdate
	case ownershipChanged:
		change.Action = ImportActionClaim
	default:
		change.Action = ImportActionNoop
		change.Status = StatusSkipped
	}
	details := []string{}
	if forcedDefinition {
		details = append(details, "forced: replaced the xcli definition while preserving target coverage")
	} else if hasExisting && configChanged {
		details = append(details, "extended target coverage")
	}
	if ownershipForced {
		details = append(details, "forced: accepted native ownership or drift")
	}
	change.Detail = strings.Join(details, "; ")
	return change
}

func candidateTargets(candidates []NativeCandidate) []string {
	seen := map[string]bool{}
	for _, candidate := range candidates {
		seen[candidate.Target] = true
	}
	result := make([]string, 0, len(seen))
	for target := range seen {
		result = append(result, target)
	}
	sort.Strings(result)
	return result
}

func cloneMCPServer(value config.MCPServer) config.MCPServer {
	value.Args = append([]string(nil), value.Args...)
	value.EnvVars = append([]string(nil), value.EnvVars...)
	value.Targets = append([]string(nil), value.Targets...)
	if value.Env != nil {
		value.Env = map[string]string{}
		for key, entry := range value.Env {
			value.Env[key] = entry
		}
	}
	return value
}

func mcpServerFingerprint(value config.MCPServer) string {
	value.Targets = nil
	value.EnvVars = sortedStrings(value.EnvVars)
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mergedTargetCoverage(existing, imported []string) []string {
	if len(existing) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, target := range existing {
		seen[target] = true
	}
	for _, target := range imported {
		seen[target] = true
	}
	result := make([]string, 0, len(seen))
	for target := range seen {
		result = append(result, target)
	}
	sort.Strings(result)
	return result
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
