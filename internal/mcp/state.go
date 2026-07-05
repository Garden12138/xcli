package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Garden12138/xcli/internal/runstore"
)

const stateVersion = 1

type Ownership struct {
	SourceConfig string `json:"source_config"`
	Fingerprint  string `json:"fingerprint"`
}

type State struct {
	Version int                             `json:"version"`
	Entries map[string]map[string]Ownership `json:"entries,omitempty"`
	path    string
}

func LoadState() (*State, error) {
	root, err := runstore.DataPath()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, "mcp-sync.json")
	state := &State{Version: stateVersion, Entries: map[string]map[string]Ownership{}, path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read MCP sync state: %w", err)
	}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("decode MCP sync state: %w", err)
	}
	if state.Version != stateVersion {
		return nil, fmt.Errorf("unsupported MCP sync state version %d", state.Version)
	}
	if state.Entries == nil {
		state.Entries = map[string]map[string]Ownership{}
	}
	state.path = path
	return state, nil
}

func (s *State) Get(target, server string) (Ownership, bool) {
	servers := s.Entries[target]
	if servers == nil {
		return Ownership{}, false
	}
	owner, ok := servers[server]
	return owner, ok
}

func (s *State) Set(target, server string, owner Ownership) {
	if s.Entries[target] == nil {
		s.Entries[target] = map[string]Ownership{}
	}
	s.Entries[target][server] = owner
}

func (s *State) Delete(target, server string) {
	delete(s.Entries[target], server)
	if len(s.Entries[target]) == 0 {
		delete(s.Entries, target)
	}
}

func (s *State) Save() error {
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
	return nil
}

func (s *State) Path() string { return s.path }

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
