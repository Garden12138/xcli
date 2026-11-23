package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"gopkg.in/yaml.v3"
)

type Duration time.Duration

func (d Duration) D() time.Duration          { return time.Duration(d) }
func (d Duration) MarshalYAML() (any, error) { return d.D().String(), nil }
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var value string
	if err := n.Decode(&value); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("请使用 20m 这样的时长值 %q：%w", value, err)
	}
	*d = Duration(parsed)
	return nil
}

type Defaults struct {
	Permission        api.Permission `yaml:"permission"`
	Network           bool           `yaml:"network"`
	InheritShellProxy bool           `yaml:"inherit_shell_proxy"`
	Review            string         `yaml:"review"` // "auto" | "off"
	Fallback          bool           `yaml:"fallback"`
	MaxFixRounds      int            `yaml:"max_fix_rounds"`
	MaxSteps          int            `yaml:"max_steps"`
	MaxParallel       int            `yaml:"max_parallel"`
	StepTimeout       Duration       `yaml:"step_timeout"`
	TaskTimeout       Duration       `yaml:"task_timeout"`
	PlannerTimeout    Duration       `yaml:"planner_timeout"`
	VerifyTimeout     Duration       `yaml:"verify_timeout"`
	Planner           string         `yaml:"planner"`
}

type AgentConfig struct {
	Command        string            `yaml:"command"`
	Enabled        *bool             `yaml:"enabled"`
	Model          string            `yaml:"model"`
	MaxConcurrency int               `yaml:"max_concurrency"`
	Env            map[string]string `yaml:"env"`
	ExtraArgs      []string          `yaml:"extra_args"`
	ArtifactDirs   []string          `yaml:"artifact_dirs"`
	Strengths      map[string]int    `yaml:"strengths"`
}

type Config struct {
	Defaults Defaults               `yaml:"defaults"`
	Path     string                 `yaml:"path"`
	Agents   map[string]AgentConfig `yaml:"agents"`
	Stats    StatsConfig            `yaml:"stats"`
}

type StatsConfig struct {
	Include []string `yaml:"include"`
}

var AgentOrder = []string{"codex", "cursor", "antigravity"}

func Default() Config {
	c := Config{Defaults: Defaults{
		Permission: api.PermWrite, Network: true, InheritShellProxy: true, Review: "auto", Fallback: true,
		MaxFixRounds: 2, MaxSteps: 6, MaxParallel: 3, Planner: "codex",
		StepTimeout: Duration(20 * time.Minute), TaskTimeout: Duration(90 * time.Minute),
		PlannerTimeout: Duration(3 * time.Minute), VerifyTimeout: Duration(10 * time.Minute),
	}, Agents: make(map[string]AgentConfig), Stats: StatsConfig{Include: []string{"chat-insight/*/xcli"}}}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = "~/.codex"
	}
	for i, name := range AgentOrder {
		enabled := true
		c.Agents[name] = AgentConfig{
			Command: []string{"codex", "cursor-agent", "agy"}[i], Enabled: &enabled, MaxConcurrency: 2,
			Env: map[string]string{}, ExtraArgs: []string{},
			ArtifactDirs: [][]string{{filepath.Join(codexHome, "generated_images")}, {"~/.cursor/projects"}, {"~/.gemini/antigravity-cli/brain"}}[i],
			Strengths: []map[string]int{
				{"code": 3, "review": 2, "ask": 2, "image": 3, "plan": 3},
				{"code": 3, "review": 3, "ask": 2, "image": 1, "plan": 1},
				{"code": 2, "review": 2, "ask": 2, "image": 2, "plan": 2},
			}[i],
		}
	}
	return c
}

func Parse(data []byte) (Config, error) {
	cfg := Default()
	defaultAgents := cfg.Agents
	cfg.Agents = nil
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("请检查配置字段：%w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("请将配置放在单个 YAML 文档中")
	}
	var raw struct {
		Agents map[string]yaml.Node `yaml:"agents"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, err
	}
	var err error
	cfg.Agents, err = mergeAgents(defaultAgents, raw.Agents)
	if err != nil {
		return Config{}, err
	}
	return cfg, validate(cfg)
}

func mergeAgents(base map[string]AgentConfig, overrides map[string]yaml.Node) (map[string]AgentConfig, error) {
	for name, node := range overrides {
		agent, ok := base[name]
		if !ok {
			return nil, fmt.Errorf("请修改 agent %q，可选 codex/cursor/antigravity", name)
		}
		if err := node.Decode(&agent); err != nil {
			return nil, err
		}
		base[name] = agent
	}
	return base, nil
}

func Load(home string) (Config, error) {
	data, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("请检查配置文件读取权限：%w", err)
	}
	return Parse(data)
}

func (c Config) Enabled(name string) bool {
	a, ok := c.Agents[name]
	return ok && (a.Enabled == nil || *a.Enabled)
}

func (c Config) EnabledAgents() []string {
	var agents []string
	for _, name := range AgentOrder {
		if c.Enabled(name) {
			agents = append(agents, name)
		}
	}
	return agents
}

func (c Config) Strength(name, kind string) int { return c.Agents[name].Strengths[kind] }
func Marshal(c Config) ([]byte, error)          { return yaml.Marshal(c) }

func validate(c Config) error {
	d := c.Defaults
	if d.Permission != api.PermRead && d.Permission != api.PermWrite && d.Permission != api.PermFull {
		return errors.New("请将 defaults.permission 设为 read/write/full")
	}
	if d.Review != "auto" && d.Review != "off" {
		return errors.New("请将 defaults.review 设为 auto/off")
	}
	if _, ok := c.Agents[d.Planner]; !ok {
		return errors.New("请将 defaults.planner 设为 codex/cursor/antigravity")
	}
	if d.MaxFixRounds < 0 {
		return errors.New("请将 defaults.max_fix_rounds 设为 0 或正整数")
	}
	for name, value := range map[string]int64{
		"max_steps": int64(d.MaxSteps), "max_parallel": int64(d.MaxParallel),
		"step_timeout": int64(d.StepTimeout), "task_timeout": int64(d.TaskTimeout),
		"planner_timeout": int64(d.PlannerTimeout), "verify_timeout": int64(d.VerifyTimeout),
	} {
		if value <= 0 {
			return fmt.Errorf("请将 defaults.%s 设为正数", name)
		}
	}
	for name, a := range c.Agents {
		if strings.TrimSpace(a.Command) == "" {
			return fmt.Errorf("请填写 agents.%s.command", name)
		}
		if a.MaxConcurrency <= 0 {
			return fmt.Errorf("请将 agents.%s.max_concurrency 设为正整数", name)
		}
		for kind, score := range a.Strengths {
			if score < 0 {
				return fmt.Errorf("请将 agents.%s.strengths.%s 设为 0 或正整数", name, kind)
			}
		}
	}
	return nil
}
