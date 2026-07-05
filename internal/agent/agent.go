package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/config"
)

type CommandSpec struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type ACPLaunch struct {
	CommandSpec
	InstallHint string
}

type Installation struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Info struct {
	Name      string `json:"name"`
	Adapter   string `json:"adapter"`
	Command   string `json:"command"`
	Network   string `json:"network,omitempty"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

type RunResult struct {
	Agent     string `json:"agent"`
	Output    string `json:"output,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	ExitCode  int    `json:"exit_code"`
	Status    string `json:"status"`
	Usage     *Usage `json:"usage,omitempty"`
}

type InstallSpec struct {
	Method  string   `json:"method"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Definition struct {
	Name   string
	Config config.AgentConfig
}

type Registry struct {
	definitions map[string]Definition
}

func NewRegistry(cfg config.Config) *Registry {
	definitions := make(map[string]Definition, len(cfg.Agents))
	for name, agentConfig := range cfg.Agents {
		definitions[name] = Definition{Name: name, Config: agentConfig}
	}
	return &Registry{definitions: definitions}
}

func (r *Registry) Get(name string) (Definition, error) {
	definition, ok := r.definitions[name]
	if !ok {
		return Definition{}, fmt.Errorf("unknown agent %q", name)
	}
	return definition, nil
}

func (r *Registry) Has(name string) bool {
	_, ok := r.definitions[name]
	return ok
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.definitions))
	for name := range r.definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (d Definition) Interactive(extra []string) CommandSpec {
	args := append([]string{}, d.Config.Args...)
	if d.Config.Adapter == "generic" {
		args = append(args, d.Config.InteractiveArgs...)
	}
	args = append(args, extra...)
	return CommandSpec{Command: d.Config.Command, Args: args}
}

func (d Definition) ACP(extra []string) (ACPLaunch, error) {
	if d.Config.ACP != nil {
		args := append([]string{}, d.Config.ACP.Args...)
		args = append(args, extra...)
		return ACPLaunch{CommandSpec: CommandSpec{Command: d.Config.ACP.Command, Args: args}}, nil
	}

	var launch ACPLaunch
	switch d.Config.Adapter {
	case "claude":
		launch.Command = "claude-agent-acp"
		launch.InstallHint = "npm install -g @agentclientprotocol/claude-agent-acp"
	case "codex":
		launch.Command = "codex-acp"
		launch.InstallHint = "npm install -g @agentclientprotocol/codex-acp"
	case "gemini":
		launch.Command = d.Config.Command
		launch.Args = append(append([]string{}, d.Config.Args...), "--acp")
	case "opencode":
		launch.Command = d.Config.Command
		launch.Args = append(append([]string{}, d.Config.Args...), "acp")
	case "generic":
		return ACPLaunch{}, fmt.Errorf("agent %q does not support ACP; configure agents.%s.acp", d.Name, d.Name)
	default:
		return ACPLaunch{}, fmt.Errorf("unsupported adapter %q", d.Config.Adapter)
	}
	launch.Args = append(launch.Args, extra...)
	return launch, nil
}

func (d Definition) Run(prompt string, structured bool, extra []string) (CommandSpec, error) {
	base := append([]string{}, d.Config.Args...)
	switch d.Config.Adapter {
	case "claude":
		base = append(base, extra...)
		base = append(base, "-p", prompt)
		if structured {
			base = append(base, "--output-format", "stream-json", "--verbose")
		}
	case "codex":
		base = append(base, "exec")
		if structured {
			base = append(base, "--json")
		}
		base = append(base, extra...)
		base = append(base, "--", prompt)
	case "gemini":
		base = append(base, extra...)
		base = append(base, "-p", prompt)
		if structured {
			base = append(base, "--output-format", "stream-json")
		}
	case "opencode":
		base = append(base, "run")
		if structured {
			base = append(base, "--format", "json")
		}
		base = append(base, extra...)
		base = append(base, prompt)
	case "generic":
		rendered, usedPrompt := renderPromptArgs(d.Config.RunArgs, prompt)
		base = append(base, rendered...)
		base = append(base, extra...)
		if !usedPrompt {
			base = append(base, prompt)
		}
	default:
		return CommandSpec{}, fmt.Errorf("unsupported adapter %q", d.Config.Adapter)
	}
	return CommandSpec{Command: d.Config.Command, Args: base}, nil
}

func (d Definition) Auth() (CommandSpec, error) {
	if len(d.Config.AuthArgs) == 0 {
		if d.Config.Adapter == "gemini" {
			return CommandSpec{Command: d.Config.Command, Args: append([]string{}, d.Config.Args...)}, nil
		}
		return CommandSpec{}, fmt.Errorf("agent %q does not define an authentication command", d.Name)
	}
	args := append([]string{}, d.Config.Args...)
	args = append(args, d.Config.AuthArgs...)
	return CommandSpec{Command: d.Config.Command, Args: args}, nil
}

func renderPromptArgs(args []string, prompt string) ([]string, bool) {
	rendered := make([]string, len(args))
	used := false
	for i, arg := range args {
		if strings.Contains(arg, "{{ prompt }}") {
			used = true
			rendered[i] = strings.ReplaceAll(arg, "{{ prompt }}", prompt)
		} else {
			rendered[i] = arg
		}
	}
	return rendered, used
}

func (d Definition) Detect(ctx context.Context) Installation {
	path, err := exec.LookPath(d.Config.Command)
	if err != nil {
		return Installation{Installed: false}
	}
	result := Installation{Installed: true, Path: path}

	versionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(versionCtx, path, "--version").CombinedOutput()
	if err != nil {
		result.Error = conciseError(err)
		return result
	}
	result.Version = versionLine(string(output))
	return result
}

func conciseError(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Sprintf("version check exited with code %d", exitErr.ExitCode())
	}
	return err.Error()
}

func versionLine(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	value = ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			value = line
		}
	}
	if len(value) > 200 {
		value = value[:200]
	}
	return value
}

func (d Definition) Install(method string) (InstallSpec, error) {
	methods := installMethods(d.Config.Adapter)
	if len(methods) == 0 {
		return InstallSpec{}, fmt.Errorf("agent %q has no built-in installer", d.Name)
	}
	if method == "" || method == "auto" {
		method = chooseAutomaticMethod(methods)
		if method == "" {
			return InstallSpec{}, errors.New("no supported package manager found; install npm or Homebrew, or select a method explicitly")
		}
	}
	spec, ok := methods[method]
	if !ok {
		available := make([]string, 0, len(methods))
		for key := range methods {
			available = append(available, key)
		}
		sort.Strings(available)
		return InstallSpec{}, fmt.Errorf("install method %q is not available for %s (available: %s)", method, d.Name, strings.Join(available, ", "))
	}
	if _, err := exec.LookPath(spec.Command); err != nil {
		return InstallSpec{}, fmt.Errorf("install method %q requires %s in PATH", method, spec.Command)
	}
	return spec, nil
}

func installMethods(adapter string) map[string]InstallSpec {
	switch adapter {
	case "claude":
		return map[string]InstallSpec{
			"npm": {Method: "npm", Command: "npm", Args: []string{"install", "-g", "@anthropic-ai/claude-code"}},
		}
	case "codex":
		return map[string]InstallSpec{
			"npm":  {Method: "npm", Command: "npm", Args: []string{"install", "-g", "@openai/codex"}},
			"brew": {Method: "brew", Command: "brew", Args: []string{"install", "--cask", "codex"}},
		}
	case "gemini":
		return map[string]InstallSpec{
			"npm":  {Method: "npm", Command: "npm", Args: []string{"install", "-g", "@google/gemini-cli"}},
			"brew": {Method: "brew", Command: "brew", Args: []string{"install", "gemini-cli"}},
		}
	case "opencode":
		return map[string]InstallSpec{
			"npm":  {Method: "npm", Command: "npm", Args: []string{"install", "-g", "opencode-ai"}},
			"brew": {Method: "brew", Command: "brew", Args: []string{"install", "anomalyco/tap/opencode"}},
		}
	default:
		return nil
	}
}

func chooseAutomaticMethod(methods map[string]InstallSpec) string {
	order := []string{"npm", "brew"}
	if runtime.GOOS == "darwin" {
		order = []string{"brew", "npm"}
	}
	for _, method := range order {
		if spec, ok := methods[method]; ok {
			if _, err := exec.LookPath(spec.Command); err == nil {
				return method
			}
		}
	}
	return ""
}

func ParseStructured(adapter, outputFormat string, data []byte) RunResult {
	result := RunResult{Status: "success"}
	if adapter == "generic" && (outputFormat == "" || outputFormat == "text") {
		result.Output = strings.TrimSpace(string(data))
		return result
	}
	if adapter == "generic" && outputFormat == "json" {
		var event map[string]interface{}
		if err := json.Unmarshal(data, &event); err == nil {
			captureSession(event, &result)
			var pieces []string
			captureOutput(event, &pieces)
			if len(pieces) > 0 {
				result.Output = strings.TrimSpace(strings.Join(pieces, ""))
				return result
			}
		}
		result.Output = strings.TrimSpace(string(data))
		return result
	}

	var pieces []string
	var lastLine string
	usage := newUsageAccumulator()
	scanner := bufio.NewScanner(bytes.NewReader(data))
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lastLine = line
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		usage.capture(adapter, event)
		captureSession(event, &result)
		captureOutput(event, &pieces)
	}
	result.Usage = usage.result()
	if len(pieces) > 0 {
		result.Output = strings.TrimSpace(strings.Join(pieces, ""))
	} else if lastLine != "" {
		result.Output = lastLine
	}
	return result
}

func captureSession(event map[string]interface{}, result *RunResult) {
	for _, key := range []string{"session_id", "sessionID", "thread_id", "threadId"} {
		if value, ok := event[key].(string); ok && value != "" {
			result.SessionID = value
		}
	}
}

func captureOutput(event map[string]interface{}, pieces *[]string) {
	typeName, _ := event["type"].(string)
	for _, key := range []string{"result", "response", "output"} {
		if value, ok := event[key].(string); ok && value != "" {
			*pieces = []string{value}
			return
		}
	}
	if item, ok := event["item"].(map[string]interface{}); ok {
		if itemType, _ := item["type"].(string); itemType == "agent_message" {
			if text, ok := item["text"].(string); ok {
				*pieces = []string{text}
				return
			}
		}
	}
	if part, ok := event["part"].(map[string]interface{}); ok {
		if text, ok := part["text"].(string); ok && (typeName == "text" || typeName == "message.part.updated") {
			*pieces = append(*pieces, text)
			return
		}
	}
	if message, ok := event["message"].(map[string]interface{}); ok {
		role, _ := message["role"].(string)
		if role == "assistant" {
			if content, ok := message["content"].(string); ok {
				*pieces = append(*pieces, content)
				return
			}
			if content, ok := message["content"].([]interface{}); ok {
				for _, entry := range content {
					if object, ok := entry.(map[string]interface{}); ok {
						if text, ok := object["text"].(string); ok {
							*pieces = append(*pieces, text)
						}
					}
				}
			}
		}
	}
	if text, ok := event["text"].(string); ok && (typeName == "message" || typeName == "assistant") {
		*pieces = append(*pieces, text)
		return
	}
	if typeName == "message" {
		role, _ := event["role"].(string)
		if role == "assistant" {
			if content, ok := event["content"].(string); ok {
				*pieces = append(*pieces, content)
			}
		}
	}
}
