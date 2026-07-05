package mcp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/Garden12138/xcli/internal/config"
	"gopkg.in/yaml.v3"
)

func ApplyImportPlan(plan *ImportPlan, state *State) error {
	if !plan.Applicable {
		return errors.New("MCP import plan contains conflicts")
	}
	updates := map[string]config.MCPServer{}
	for _, change := range plan.Changes {
		if (change.Action == ImportActionAdd || change.Action == ImportActionUpdate) && change.desired != nil {
			updates[change.Server] = cloneMCPServer(*change.desired)
		}
	}

	original, readErr := os.ReadFile(plan.SourceConfig)
	existed := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read xcli source configuration: %w", readErr)
	}
	if !existed && plan.Scope == ScopeProject {
		return errors.New("project MCP import requires an existing xcli source configuration")
	}
	if plan.verifySource && (existed != plan.sourceExists || existed && contentFingerprint(original) != plan.sourceFingerprint) {
		return errors.New("xcli source configuration changed after the import plan was built; rerun the command")
	}
	for path, fingerprint := range plan.nativeFiles {
		data, err := os.ReadFile(path)
		if fingerprint == "" && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || contentFingerprint(data) != fingerprint {
			return fmt.Errorf("native MCP configuration %s changed after the import plan was built; rerun the command", path)
		}
	}
	mode := os.FileMode(0o600)
	if existed {
		info, err := os.Stat(plan.SourceConfig)
		if err != nil {
			return err
		}
		mode = info.Mode().Perm()
	}
	wroteConfig := false
	if len(updates) > 0 {
		patched, err := patchMCPServers(original, updates)
		if err != nil {
			return fmt.Errorf("update xcli MCP source configuration: %w", err)
		}
		if _, err := config.Decode(patched, plan.SourceConfig); err != nil {
			return err
		}
		if err := writeAtomic(plan.SourceConfig, patched, mode); err != nil {
			return fmt.Errorf("write xcli source configuration: %w", err)
		}
		wroteConfig = true
	}

	namespace := NamespaceKey(plan.Scope, plan.ProjectDir)
	previous := map[string]map[string]previousOwnership{}
	for _, change := range plan.Changes {
		if change.Action != ImportActionAdd && change.Action != ImportActionUpdate && change.Action != ImportActionClaim {
			continue
		}
		for target, fingerprint := range change.ownership {
			if previous[target] == nil {
				previous[target] = map[string]previousOwnership{}
			}
			owner, owned := state.GetIn(namespace, target, change.Server)
			previous[target][change.Server] = previousOwnership{owner: owner, owned: owned}
			state.SetIn(namespace, target, change.Server, Ownership{
				SourceConfig: plan.SourceConfig,
				Fingerprint:  fingerprint.Fingerprint(),
			})
		}
	}
	if err := state.Save(); err != nil {
		restoreOwnership(state, namespace, previous)
		if wroteConfig {
			var restoreErr error
			if existed {
				restoreErr = writeAtomic(plan.SourceConfig, original, mode)
			} else {
				restoreErr = os.Remove(plan.SourceConfig)
			}
			if restoreErr != nil {
				return fmt.Errorf("save MCP ownership: %v; restore source configuration: %w", err, restoreErr)
			}
		}
		return err
	}
	for index := range plan.Changes {
		change := &plan.Changes[index]
		if change.Action == ImportActionAdd || change.Action == ImportActionUpdate || change.Action == ImportActionClaim {
			change.Status = StatusApplied
		}
	}
	plan.Applied = true
	return nil
}

type previousOwnership struct {
	owner Ownership
	owned bool
}

func restoreOwnership(state *State, namespace string, previous map[string]map[string]previousOwnership) {
	for target, servers := range previous {
		for server, value := range servers {
			if value.owned {
				state.SetIn(namespace, target, server, value.owner)
			} else {
				state.DeleteIn(namespace, target, server)
			}
		}
	}
}

func patchMCPServers(original []byte, updates map[string]config.MCPServer) ([]byte, error) {
	var document yaml.Node
	if len(bytes.TrimSpace(original)) == 0 {
		document = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
		root := document.Content[0]
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "version"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"},
		)
	} else {
		decoder := yaml.NewDecoder(bytes.NewReader(original))
		if err := decoder.Decode(&document); err != nil {
			return nil, err
		}
		var extra yaml.Node
		if err := decoder.Decode(&extra); err == nil {
			return nil, errors.New("multiple YAML documents are not supported")
		} else if err != io.EOF {
			return nil, err
		}
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("configuration root must be a mapping")
	}
	root := document.Content[0]
	mcpNode, err := ensureYAMLMapping(root, "mcp")
	if err != nil {
		return nil, err
	}
	serversNode, err := ensureYAMLMapping(mcpNode, "servers")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(updates))
	for name := range updates {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		var value yaml.Node
		if err := value.Encode(updates[name]); err != nil {
			return nil, err
		}
		index := yamlMappingIndex(serversNode, name)
		if index >= 0 {
			old := serversNode.Content[index+1]
			mergeYAMLComments(old, &value)
			serversNode.Content[index+1] = &value
		} else {
			serversNode.Content = append(serversNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
				&value,
			)
		}
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func mergeYAMLComments(old, updated *yaml.Node) {
	updated.HeadComment = old.HeadComment
	updated.LineComment = old.LineComment
	updated.FootComment = old.FootComment
	if old.Kind != yaml.MappingNode || updated.Kind != yaml.MappingNode {
		return
	}
	for index := 0; index+1 < len(updated.Content); index += 2 {
		key := updated.Content[index].Value
		oldIndex := yamlMappingIndex(old, key)
		if oldIndex < 0 {
			continue
		}
		updated.Content[index].HeadComment = old.Content[oldIndex].HeadComment
		updated.Content[index].LineComment = old.Content[oldIndex].LineComment
		updated.Content[index].FootComment = old.Content[oldIndex].FootComment
		mergeYAMLComments(old.Content[oldIndex+1], updated.Content[index+1])
	}
}

func ensureYAMLMapping(parent *yaml.Node, key string) (*yaml.Node, error) {
	index := yamlMappingIndex(parent, key)
	if index >= 0 {
		value := parent.Content[index+1]
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s must be a mapping", key)
		}
		return value, nil
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content, keyNode, valueNode)
	return valueNode, nil
}

func yamlMappingIndex(mapping *yaml.Node, key string) int {
	if mapping.Kind != yaml.MappingNode {
		return -1
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return index
		}
	}
	return -1
}
