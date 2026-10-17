package agent

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/config"
)

type CommandSpec struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Installation struct {
	Installed      bool                  `json:"installed"`
	Path           string                `json:"path,omitempty"`
	Version        string                `json:"version,omitempty"`
	Error          string                `json:"error,omitempty"`
	Capabilities   map[string]Capability `json:"capabilities,omitempty"`
	Authentication string                `json:"authentication,omitempty"`
}

type Info struct {
	Name         string                `json:"name"`
	Adapter      string                `json:"adapter"`
	Command      string                `json:"command"`
	Network      string                `json:"network,omitempty"`
	Installed    bool                  `json:"installed"`
	Path         string                `json:"path,omitempty"`
	Version      string                `json:"version,omitempty"`
	Error        string                `json:"error,omitempty"`
	Capabilities map[string]Capability `json:"capabilities,omitempty"`
}

type RunResult struct {
	RetryNotices int    `json:"retry_notices,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	ParseStatus  string `json:"parse_status,omitempty"`
	ErrorKind    string `json:"error_kind,omitempty"`
	Error        string `json:"error,omitempty"`
	Agent        string `json:"agent"`
	Output       string `json:"output,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	ExitCode     int    `json:"exit_code"`
	Status       string `json:"status"`
	Usage        *Usage `json:"usage,omitempty"`
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

func (d Definition) Run(prompt string, structured bool, extra []string) (CommandSpec, error) {
	base := append([]string{}, d.Config.Args...)
	switch d.Config.Adapter {
	case "cursor", "antigravity":
		base = append(base, extra...)
		if structured {
			base = append(base, "--output-format", "stream-json")
		}
		if d.Config.Adapter == "antigravity" {
			base = append(base, "--print-timeout", "120s")
		}
		base = append(base, "--print", prompt)
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

func (d Definition) Resume(sessionID, prompt string, structured bool, extra []string) (CommandSpec, error) {
	if strings.TrimSpace(sessionID) == "" {
		return CommandSpec{}, errors.New("a session id is required")
	}

	base := append([]string{}, d.Config.Args...)
	interactive := prompt == ""
	switch d.Config.Adapter {
	case "cursor", "antigravity":
		flag := "--resume"
		if d.Config.Adapter == "antigravity" {
			flag = "--conversation"
		}
		base = append(base, extra...)
		base = append(base, flag, sessionID)
		if !interactive {
			if structured {
				base = append(base, "--output-format", "stream-json")
			}
			if d.Config.Adapter == "antigravity" {
				base = append(base, "--print-timeout", "120s")
			}
			base = append(base, "--print", prompt)
		}
	case "claude":
		base = append(base, extra...)
		base = append(base, "--resume", sessionID)
		if !interactive {
			base = append(base, "-p", prompt)
			if structured {
				base = append(base, "--output-format", "stream-json", "--verbose")
			}
		}
	case "codex":
		if interactive {
			base = append(base, "resume")
			base = append(base, extra...)
			base = append(base, sessionID)
		} else {
			base = append(base, "exec", "resume")
			if structured {
				base = append(base, "--json")
			}
			base = append(base, extra...)
			base = append(base, sessionID, prompt)
		}
	case "gemini":
		base = append(base, extra...)
		base = append(base, "--resume", sessionID)
		if !interactive {
			base = append(base, "-p", prompt)
			if structured {
				base = append(base, "--output-format", "stream-json")
			}
		}
	case "opencode":
		if interactive {
			base = append(base, extra...)
			base = append(base, "--session", sessionID)
		} else {
			base = append(base, "run")
			if structured {
				base = append(base, "--format", "json")
			}
			base = append(base, extra...)
			base = append(base, "--session", sessionID, prompt)
		}
	case "generic":
		return CommandSpec{}, fmt.Errorf("agent %q does not support session resume", d.Name)
	default:
		return CommandSpec{}, fmt.Errorf("unsupported adapter %q", d.Config.Adapter)
	}
	return CommandSpec{Command: d.Config.Command, Args: base}, nil
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
	result := Installation{Installed: true, Path: path, Capabilities: d.Capabilities(), Authentication: "unknown"}

	versionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(versionCtx, path, "--version")
	command.WaitDelay = 250 * time.Millisecond
	var output probeOutput
	command.Stdout = &output
	command.Stderr = &output
	err = command.Run()
	if err != nil {
		if versionCtx.Err() != nil {
			result.Error = "version check timed out"
		} else {
			result.Error = conciseError(err)
		}
		return result
	}
	if output.truncated {
		result.Error = "version output exceeds 64 KiB"
		return result
	}
	result.Version = versionLine(string(output.data))
	if result.Version == "" {
		result.Error = "version check returned no version"
	}
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

func ParseStructured(adapter, outputFormat string, data []byte) RunResult {
	decoder := NewStreamDecoder(adapter, outputFormat, nil)
	_, _ = decoder.Write(data)
	return decoder.Finish()
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
