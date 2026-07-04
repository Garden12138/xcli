package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const CurrentVersion = 1

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Config struct {
	Version      int                    `yaml:"version" json:"version"`
	DefaultAgent string                 `yaml:"default_agent,omitempty" json:"default_agent,omitempty"`
	Agents       map[string]AgentConfig `yaml:"agents,omitempty" json:"agents,omitempty"`
	Networks     map[string]Network     `yaml:"networks,omitempty" json:"networks,omitempty"`
	Routing      Routing                `yaml:"routing,omitempty" json:"routing,omitempty"`
	Recording    Recording              `yaml:"recording,omitempty" json:"recording"`
}

type Routing struct {
	Rules []RouteRule `yaml:"rules,omitempty" json:"rules,omitempty"`
}

type RouteRule struct {
	Name        string `yaml:"name" json:"name"`
	PromptRegex string `yaml:"prompt_regex" json:"prompt_regex"`
	Agent       string `yaml:"agent" json:"agent"`
}

type AgentConfig struct {
	Adapter         string            `yaml:"adapter,omitempty" json:"adapter,omitempty"`
	Command         string            `yaml:"command,omitempty" json:"command,omitempty"`
	Network         string            `yaml:"network,omitempty" json:"network,omitempty"`
	Args            []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env             map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	InteractiveArgs []string          `yaml:"interactive_args,omitempty" json:"interactive_args,omitempty"`
	RunArgs         []string          `yaml:"run_args,omitempty" json:"run_args,omitempty"`
	AuthArgs        []string          `yaml:"auth_args,omitempty" json:"auth_args,omitempty"`
	Output          string            `yaml:"output,omitempty" json:"output,omitempty"`
}

type Network struct {
	Set   map[string]string `yaml:"set,omitempty" json:"set,omitempty"`
	Unset []string          `yaml:"unset,omitempty" json:"unset,omitempty"`
}

type Recording struct {
	Output bool `yaml:"output,omitempty" json:"output"`
}

func Defaults() Config {
	return Config{
		Version: CurrentVersion,
		Agents: map[string]AgentConfig{
			"claude": {
				Adapter:  "claude",
				Command:  "claude",
				AuthArgs: []string{"auth", "login"},
			},
			"codex": {
				Adapter:  "codex",
				Command:  "codex",
				AuthArgs: []string{"login"},
			},
			"gemini": {
				Adapter: "gemini",
				Command: "gemini",
			},
			"opencode": {
				Adapter:  "opencode",
				Command:  "opencode",
				AuthArgs: []string{"auth", "login"},
			},
		},
		Networks:  map[string]Network{},
		Recording: Recording{},
	}
}

func DefaultPath() (string, error) {
	if root := os.Getenv("XDG_CONFIG_HOME"); root != "" {
		return filepath.Join(root, "xcli", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "xcli", "config.yaml"), nil
}

func ResolvePath(explicit string) (string, error) {
	if explicit != "" {
		path, err := filepath.Abs(explicit)
		if err != nil {
			return "", fmt.Errorf("resolve config path: %w", err)
		}
		return path, nil
	}
	return DefaultPath()
}

func Load(explicit string) (Config, string, error) {
	path, err := ResolvePath(explicit)
	if err != nil {
		return Config{}, "", err
	}

	base := Defaults()
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := base.Validate(); err != nil {
			return Config{}, path, err
		}
		return base, path, nil
	}
	if err != nil {
		return Config{}, path, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	var user Config
	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	if err := decoder.Decode(&user); err != nil {
		return Config{}, path, fmt.Errorf("decode config %s: %w", path, err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, path, fmt.Errorf("decode config %s: multiple YAML documents are not supported", path)
		}
		return Config{}, path, fmt.Errorf("decode config %s: %w", path, err)
	}

	merged := merge(base, user)
	if err := merged.Validate(); err != nil {
		return Config{}, path, fmt.Errorf("validate config %s: %w", path, err)
	}
	return merged, path, nil
}

func merge(base, user Config) Config {
	if user.Version != 0 {
		base.Version = user.Version
	}
	if user.DefaultAgent != "" {
		base.DefaultAgent = user.DefaultAgent
	}
	for name, override := range user.Agents {
		current, exists := base.Agents[name]
		if !exists {
			current = AgentConfig{}
		}
		if override.Adapter != "" {
			current.Adapter = override.Adapter
		}
		if override.Command != "" {
			current.Command = override.Command
		}
		if override.Network != "" {
			current.Network = override.Network
		}
		if override.Args != nil {
			current.Args = append([]string(nil), override.Args...)
		}
		if override.Env != nil {
			current.Env = cloneMap(override.Env)
		}
		if override.InteractiveArgs != nil {
			current.InteractiveArgs = append([]string(nil), override.InteractiveArgs...)
		}
		if override.RunArgs != nil {
			current.RunArgs = append([]string(nil), override.RunArgs...)
		}
		if override.AuthArgs != nil {
			current.AuthArgs = append([]string(nil), override.AuthArgs...)
		}
		if override.Output != "" {
			current.Output = override.Output
		}
		base.Agents[name] = current
	}
	if user.Networks != nil {
		base.Networks = make(map[string]Network, len(user.Networks))
		for name, network := range user.Networks {
			base.Networks[name] = Network{Set: cloneMap(network.Set), Unset: append([]string(nil), network.Unset...)}
		}
	}
	if user.Routing.Rules != nil {
		base.Routing.Rules = append([]RouteRule(nil), user.Routing.Rules...)
	}
	base.Recording = user.Recording
	return base
}

func cloneMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func (c Config) Validate() error {
	if c.Version != CurrentVersion {
		return fmt.Errorf("unsupported config version %d (expected %d)", c.Version, CurrentVersion)
	}
	if len(c.Agents) == 0 {
		return errors.New("at least one agent must be configured")
	}
	if c.DefaultAgent != "" {
		if _, ok := c.Agents[c.DefaultAgent]; !ok {
			return fmt.Errorf("default agent %q is not configured", c.DefaultAgent)
		}
	}
	for name, agent := range c.Agents {
		if !namePattern.MatchString(name) {
			return fmt.Errorf("invalid agent name %q", name)
		}
		if agent.Command == "" {
			return fmt.Errorf("agent %q has no command", name)
		}
		switch agent.Adapter {
		case "claude", "codex", "gemini", "opencode":
		case "generic":
			if len(agent.RunArgs) == 0 {
				return fmt.Errorf("generic agent %q must define run_args", name)
			}
		default:
			return fmt.Errorf("agent %q uses unsupported adapter %q", name, agent.Adapter)
		}
		if agent.Output != "" && agent.Output != "text" && agent.Output != "json" && agent.Output != "jsonl" {
			return fmt.Errorf("agent %q has invalid output format %q", name, agent.Output)
		}
		if agent.Network != "" {
			if _, ok := c.Networks[agent.Network]; !ok {
				return fmt.Errorf("agent %q references missing network %q", name, agent.Network)
			}
		}
		for _, arg := range append(append([]string{}, agent.RunArgs...), agent.InteractiveArgs...) {
			if err := validateArgumentTemplate(arg); err != nil {
				return fmt.Errorf("agent %q: %w", name, err)
			}
		}
	}
	seenRules := make(map[string]bool, len(c.Routing.Rules))
	for index, rule := range c.Routing.Rules {
		label := fmt.Sprintf("routing rule %d", index+1)
		if !namePattern.MatchString(rule.Name) {
			return fmt.Errorf("%s has invalid name %q", label, rule.Name)
		}
		if seenRules[rule.Name] {
			return fmt.Errorf("duplicate routing rule name %q", rule.Name)
		}
		seenRules[rule.Name] = true
		if strings.TrimSpace(rule.PromptRegex) == "" {
			return fmt.Errorf("routing rule %q has an empty prompt_regex", rule.Name)
		}
		if _, err := regexp.Compile(rule.PromptRegex); err != nil {
			return fmt.Errorf("routing rule %q has invalid prompt_regex: %w", rule.Name, err)
		}
		if _, ok := c.Agents[rule.Agent]; !ok {
			return fmt.Errorf("routing rule %q references unknown agent %q", rule.Name, rule.Agent)
		}
	}
	for name, network := range c.Networks {
		if !namePattern.MatchString(name) {
			return fmt.Errorf("invalid network name %q", name)
		}
		seen := map[string]bool{}
		for _, key := range network.Unset {
			if key == "" || strings.ContainsRune(key, '=') {
				return fmt.Errorf("network %q contains invalid unset key %q", name, key)
			}
			if seen[key] {
				return fmt.Errorf("network %q repeats unset key %q", name, key)
			}
			seen[key] = true
		}
		for key := range network.Set {
			if key == "" || strings.ContainsRune(key, '=') {
				return fmt.Errorf("network %q contains invalid set key %q", name, key)
			}
		}
	}
	return nil
}

func validateArgumentTemplate(value string) error {
	remaining := strings.ReplaceAll(value, "{{ prompt }}", "")
	if strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}") {
		return fmt.Errorf("unsupported argument template %q; only {{ prompt }} is allowed", value)
	}
	return nil
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := writeAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".xcli-*")
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

func SortedAgentNames(c Config) []string {
	names := make([]string, 0, len(c.Agents))
	for name := range c.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
