package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Garden12138/xcli/internal/runstore"
)

const stateVersion = 2

type Ownership struct {
	SourceConfig string `json:"source_config"`
	Fingerprint  string `json:"fingerprint"`
}

type OwnershipNamespace struct {
	Scope      string                          `json:"scope"`
	ProjectDir string                          `json:"project_dir,omitempty"`
	Entries    map[string]map[string]Ownership `json:"entries,omitempty"`
}

type State struct {
	Version    int                            `json:"version"`
	Namespaces map[string]*OwnershipNamespace `json:"namespaces,omitempty"`
	path       string
	migrated   bool
}

func LoadState() (*State, error) {
	root, err := runstore.DataPath()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, "mcp-sync.json")
	state := newState(path)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read MCP sync state: %w", err)
	}
	var version struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return nil, fmt.Errorf("decode MCP sync state: %w", err)
	}
	switch version.Version {
	case 1:
		var legacy struct {
			Version int                             `json:"version"`
			Entries map[string]map[string]Ownership `json:"entries,omitempty"`
		}
		if err := json.Unmarshal(data, &legacy); err != nil {
			return nil, fmt.Errorf("decode MCP sync state: %w", err)
		}
		state.Namespaces[ScopeUser] = &OwnershipNamespace{
			Scope: ScopeUser, Entries: legacy.Entries,
		}
		if state.Namespaces[ScopeUser].Entries == nil {
			state.Namespaces[ScopeUser].Entries = map[string]map[string]Ownership{}
		}
		state.migrated = true
	case stateVersion:
		if err := json.Unmarshal(data, state); err != nil {
			return nil, fmt.Errorf("decode MCP sync state: %w", err)
		}
		if state.Namespaces == nil {
			state.Namespaces = map[string]*OwnershipNamespace{}
		}
	default:
		return nil, fmt.Errorf("unsupported MCP sync state version %d", version.Version)
	}
	state.Version = stateVersion
	state.path = path
	return state, nil
}

func newState(path string) *State {
	return &State{Version: stateVersion, Namespaces: map[string]*OwnershipNamespace{}, path: path}
}

func NamespaceKey(scope, projectDir string) string {
	if scope == ScopeProject {
		return ScopeProject + ":" + projectDir
	}
	return ScopeUser
}

func (s *State) namespace(key string, create bool) *OwnershipNamespace {
	if s.Namespaces == nil {
		if !create {
			return nil
		}
		s.Namespaces = map[string]*OwnershipNamespace{}
	}
	value := s.Namespaces[key]
	if value == nil && create {
		value = &OwnershipNamespace{Scope: ScopeUser, Entries: map[string]map[string]Ownership{}}
		if strings.HasPrefix(key, ScopeProject+":") {
			value.Scope = ScopeProject
			value.ProjectDir = strings.TrimPrefix(key, ScopeProject+":")
		}
		s.Namespaces[key] = value
	}
	if value != nil && value.Entries == nil {
		value.Entries = map[string]map[string]Ownership{}
	}
	return value
}

func (s *State) EntriesFor(key string) map[string]map[string]Ownership {
	value := s.namespace(key, false)
	if value == nil {
		return nil
	}
	return value.Entries
}

func (s *State) GetIn(key, target, server string) (Ownership, bool) {
	value := s.namespace(key, false)
	if value == nil {
		return Ownership{}, false
	}
	servers := value.Entries[target]
	if servers == nil {
		return Ownership{}, false
	}
	owner, ok := servers[server]
	return owner, ok
}

func (s *State) SetIn(key, target, server string, owner Ownership) {
	value := s.namespace(key, true)
	if value.Entries[target] == nil {
		value.Entries[target] = map[string]Ownership{}
	}
	value.Entries[target][server] = owner
}

func (s *State) DeleteIn(key, target, server string) {
	value := s.namespace(key, false)
	if value == nil {
		return
	}
	delete(value.Entries[target], server)
	if len(value.Entries[target]) == 0 {
		delete(value.Entries, target)
	}
	if len(value.Entries) == 0 {
		delete(s.Namespaces, key)
	}
}

func (s *State) Get(target, server string) (Ownership, bool) {
	return s.GetIn(ScopeUser, target, server)
}

func (s *State) Set(target, server string, owner Ownership) {
	s.SetIn(ScopeUser, target, server, owner)
}

func (s *State) Delete(target, server string) { s.DeleteIn(ScopeUser, target, server) }

func (s *State) Save() error {
	s.Version = stateVersion
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create MCP state directory: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode MCP sync state: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomic(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write MCP sync state: %w", err)
	}
	s.migrated = false
	return nil
}

func (s *State) Path() string { return s.path }

func (s *State) NeedsSave() bool { return s.migrated }

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".xcli-mcp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
