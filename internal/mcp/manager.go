package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/tailscale/hujson"
)

type Manager interface {
	Load(context.Context) (map[string]Entry, error)
	Apply(context.Context, Change) error
}

type commandManager struct {
	target      string
	config      config.AgentConfig
	environment []string
	configPath  string
}

func NewManagers(cfg config.Config) (map[string]Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	result := map[string]Manager{}
	for _, target := range []string{"claude", "codex", "gemini"} {
		agentConfig := cfg.Agents[target]
		environment, err := xruntime.BuildEnvironment(os.Environ(), cfg, agentConfig, "")
		if err != nil {
			return nil, err
		}
		path := ""
		switch target {
		case "claude":
			path = filepath.Join(home, ".claude.json")
		case "gemini":
			path = filepath.Join(home, ".gemini", "settings.json")
		}
		result[target] = &commandManager{target: target, config: agentConfig, environment: environment, configPath: path}
	}
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		root = filepath.Join(home, ".config")
	}
	result["opencode"] = &openCodeManager{path: filepath.Join(root, "opencode", "opencode.json")}
	return result, nil
}

func (m *commandManager) Preflight(ctx context.Context) error {
	args := append([]string{}, m.config.Args...)
	args = append(args, "mcp", "--help")
	_, err := runCommand(ctx, agent.CommandSpec{Command: m.config.Command, Args: args}, m.environment)
	return err
}

func (m *commandManager) Load(ctx context.Context) (map[string]Entry, error) {
	if m.target == "codex" {
		args := append([]string{}, m.config.Args...)
		args = append(args, "mcp", "list", "--json")
		stdout, err := runCommand(ctx, agent.CommandSpec{Command: m.config.Command, Args: args}, m.environment)
		if err != nil {
			return nil, err
		}
		return parseCodexEntries(stdout)
	}
	data, err := os.ReadFile(m.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if m.target == "claude" {
		return parseJSONEntries(data, "mcpServers", parseClaudeEntry)
	}
	return parseJSONEntries(data, "mcpServers", parseGeminiEntry)
}

func (m *commandManager) Apply(ctx context.Context, change Change) error {
	switch change.Action {
	case ActionAdd:
		return m.add(ctx, change.Server, *change.desired)
	case ActionRemove:
		return m.remove(ctx, change.Server)
	case ActionUpdate:
		if err := m.remove(ctx, change.Server); err != nil {
			return err
		}
		if err := m.add(ctx, change.Server, *change.desired); err != nil {
			if change.current != nil {
				if restoreErr := m.add(ctx, change.Server, *change.current); restoreErr != nil {
					return fmt.Errorf("update failed: %v; restoring previous entry failed: %v", err, restoreErr)
				}
			}
			return err
		}
		return nil
	default:
		return nil
	}
}

func (m *commandManager) add(ctx context.Context, name string, entry Entry) error {
	args := append([]string{}, m.config.Args...)
	switch m.target {
	case "claude":
		definition := map[string]interface{}{"type": entry.Transport}
		if entry.Transport == "stdio" {
			definition["command"] = entry.Command
			definition["args"] = entry.Args
			if len(entry.EnvVars) > 0 {
				environment := map[string]string{}
				for _, key := range entry.EnvVars {
					environment[key] = "${" + key + "}"
				}
				definition["env"] = environment
			}
		} else {
			definition["url"] = entry.URL
		}
		data, _ := json.Marshal(definition)
		args = append(args, "mcp", "add-json", "--scope", "user", name, string(data))
	case "codex":
		args = append(args, "mcp", "add", name)
		if entry.Transport == "http" {
			args = append(args, "--url", entry.URL)
		} else {
			args = append(args, "--", entry.Command)
			args = append(args, entry.Args...)
		}
	case "gemini":
		args = append(args, "mcp", "add", "--scope", "user")
		for _, key := range entry.EnvVars {
			args = append(args, "--env", key+"=$"+key)
		}
		if entry.Transport == "http" {
			args = append(args, "--transport", "http", name, entry.URL)
		} else {
			args = append(args, name, entry.Command)
			args = append(args, entry.Args...)
		}
	}
	_, err := runCommand(ctx, agent.CommandSpec{Command: m.config.Command, Args: args}, m.environment)
	if err != nil {
		return err
	}
	if m.target == "codex" && entry.Transport == "stdio" && len(entry.EnvVars) > 0 {
		if err := m.setCodexEnvVars(name, entry.EnvVars); err != nil {
			_ = m.remove(ctx, name)
			return err
		}
	}
	return nil
}

func (m *commandManager) remove(ctx context.Context, name string) error {
	args := append([]string{}, m.config.Args...)
	args = append(args, "mcp", "remove")
	if m.target == "claude" || m.target == "gemini" {
		args = append(args, "--scope", "user")
	}
	args = append(args, name)
	_, err := runCommand(ctx, agent.CommandSpec{Command: m.config.Command, Args: args}, m.environment)
	return err
}

func runCommand(ctx context.Context, spec agent.CommandSpec, environment []string) ([]byte, error) {
	var stderr bytes.Buffer
	result, err := xruntime.RunProcess(ctx, spec, xruntime.ProcessOptions{
		Env: environment, Stderr: &stderr, CaptureStdout: true,
	})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = fmt.Sprintf("command exited with code %d", result.ExitCode)
		}
		return nil, errors.New(message)
	}
	return result.Stdout, nil
}

func parseJSONEntries(data []byte, key string, parse func(json.RawMessage) (Entry, bool)) (map[string]Entry, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var servers map[string]json.RawMessage
	if raw := root[key]; raw != nil {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, err
		}
	}
	result := map[string]Entry{}
	for name, raw := range servers {
		if entry, ok := parse(raw); ok {
			result[name] = entry
		} else {
			// Preserve name occupancy even when a vendor entry uses fields that
			// xcli does not understand. A desired server with the same name must
			// conflict instead of silently taking it over.
			result[name] = Entry{Transport: "unknown"}
		}
	}
	return result, nil
}

func parseClaudeEntry(raw json.RawMessage) (Entry, bool) {
	var value struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		URL     string            `json:"url"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return Entry{}, false
	}
	if value.Command != "" || value.Type == "stdio" {
		return Entry{Transport: "stdio", Command: value.Command, Args: value.Args, EnvVars: referencedEnvVars(value.Env, "${%s}")}, true
	}
	if value.URL != "" && (value.Type == "http" || value.Type == "streamable-http") {
		return Entry{Transport: "http", URL: value.URL}, true
	}
	return Entry{}, false
}

func parseGeminiEntry(raw json.RawMessage) (Entry, bool) {
	var value struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		HTTPURL string            `json:"httpUrl"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return Entry{}, false
	}
	if value.Command != "" {
		return Entry{Transport: "stdio", Command: value.Command, Args: value.Args, EnvVars: referencedEnvVars(value.Env, "$%s")}, true
	}
	if value.HTTPURL != "" {
		return Entry{Transport: "http", URL: value.HTTPURL}, true
	}
	return Entry{}, false
}

func parseCodexEntries(data []byte) (map[string]Entry, error) {
	var values []struct {
		Name      string `json:"name"`
		Transport struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
			EnvVars []string `json:"env_vars"`
			URL     string   `json:"url"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	result := map[string]Entry{}
	for _, value := range values {
		switch value.Transport.Type {
		case "stdio":
			result[value.Name] = Entry{Transport: "stdio", Command: value.Transport.Command, Args: value.Transport.Args, EnvVars: sortedStrings(value.Transport.EnvVars)}
		case "streamable_http", "http":
			result[value.Name] = Entry{Transport: "http", URL: value.Transport.URL}
		default:
			result[value.Name] = Entry{Transport: "unknown"}
		}
	}
	return result, nil
}

type openCodeManager struct{ path string }

func (m *openCodeManager) Load(context.Context) (map[string]Entry, error) {
	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	standard, err := hujson.Standardize(data)
	if err != nil {
		return nil, err
	}
	return parseJSONEntries(standard, "mcp", parseOpenCodeEntry)
}

func parseOpenCodeEntry(raw json.RawMessage) (Entry, bool) {
	var value struct {
		Type        string            `json:"type"`
		Command     []string          `json:"command"`
		Environment map[string]string `json:"environment"`
		URL         string            `json:"url"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return Entry{}, false
	}
	if value.Type == "local" && len(value.Command) > 0 {
		return Entry{Transport: "stdio", Command: value.Command[0], Args: value.Command[1:], EnvVars: referencedEnvVars(value.Environment, "{env:%s}")}, true
	}
	if value.Type == "remote" && value.URL != "" {
		return Entry{Transport: "http", URL: value.URL}, true
	}
	return Entry{}, false
}

func (m *openCodeManager) Apply(_ context.Context, change Change) error {
	data, err := os.ReadFile(m.path)
	exists := err == nil
	if errors.Is(err, os.ErrNotExist) {
		data = []byte("{}\n")
	} else if err != nil {
		return err
	}
	value, err := hujson.Parse(data)
	if err != nil {
		return err
	}
	if err := updateOpenCodeDocument(&value, change); err != nil {
		return err
	}
	if exists {
		info, statErr := os.Stat(m.path)
		mode := os.FileMode(0o600)
		if statErr == nil {
			mode = info.Mode().Perm()
		}
		if err := writeAtomic(m.path+".xcli.bak", data, mode); err != nil {
			return fmt.Errorf("back up OpenCode config: %w", err)
		}
	}
	return writeAtomic(m.path, value.Pack(), 0o600)
}

func updateOpenCodeDocument(document *hujson.Value, change Change) error {
	root, ok := document.Value.(*hujson.Object)
	if !ok {
		return errors.New("OpenCode config root must be an object")
	}
	mcpValue := document.Find("/mcp")
	if mcpValue == nil {
		if change.Action == ActionRemove {
			return fmt.Errorf("OpenCode MCP server %q does not exist", change.Server)
		}
		member, err := newObjectMember("mcp", map[string]interface{}{change.Server: openCodeValue(*change.desired)})
		if err != nil {
			return err
		}
		root.Members = append(root.Members, member)
		return nil
	}
	mcpObject, ok := mcpValue.Value.(*hujson.Object)
	if !ok {
		return errors.New("OpenCode mcp configuration must be an object")
	}
	index := objectMemberIndex(mcpObject, change.Server)
	if change.Action == ActionRemove {
		if index < 0 {
			return fmt.Errorf("OpenCode MCP server %q does not exist", change.Server)
		}
		mcpObject.Members = append(mcpObject.Members[:index], mcpObject.Members[index+1:]...)
		return nil
	}
	member, err := newObjectMember(change.Server, openCodeValue(*change.desired))
	if err != nil {
		return err
	}
	if index >= 0 {
		member.Name.BeforeExtra = mcpObject.Members[index].Name.BeforeExtra
		member.Value.AfterExtra = mcpObject.Members[index].Value.AfterExtra
		mcpObject.Members[index] = member
	} else {
		mcpObject.Members = append(mcpObject.Members, member)
	}
	return nil
}

func newObjectMember(name string, value interface{}) (hujson.ObjectMember, error) {
	data, err := json.Marshal(map[string]interface{}{name: value})
	if err != nil {
		return hujson.ObjectMember{}, err
	}
	parsed, err := hujson.Parse(data)
	if err != nil {
		return hujson.ObjectMember{}, err
	}
	object, ok := parsed.Value.(*hujson.Object)
	if !ok || len(object.Members) != 1 {
		return hujson.ObjectMember{}, errors.New("encode OpenCode MCP entry")
	}
	return object.Members[0], nil
}

func objectMemberIndex(object *hujson.Object, name string) int {
	for index, member := range object.Members {
		standard, err := hujson.Standardize(member.Name.Pack())
		if err != nil {
			continue
		}
		var key string
		if json.Unmarshal(standard, &key) == nil && key == name {
			return index
		}
	}
	return -1
}

func openCodeValue(entry Entry) map[string]interface{} {
	if entry.Transport == "http" {
		return map[string]interface{}{"type": "remote", "url": entry.URL}
	}
	command := append([]string{entry.Command}, entry.Args...)
	value := map[string]interface{}{"type": "local", "command": command}
	if len(entry.EnvVars) > 0 {
		environment := map[string]string{}
		for _, key := range entry.EnvVars {
			environment[key] = "{env:" + key + "}"
		}
		value["environment"] = environment
	}
	return value
}

func referencedEnvVars(environment map[string]string, format string) []string {
	result := []string{}
	for key, value := range environment {
		if value == fmt.Sprintf(format, key) || (format == "$%s" && value == "${"+key+"}") {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func (m *commandManager) setCodexEnvVars(name string, variables []string) error {
	values := environmentMap(m.environment)
	root := values["CODEX_HOME"]
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		root = filepath.Join(home, ".codex")
	}
	path := filepath.Join(root, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Codex config for env_vars: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	start, end := -1, len(lines)
	for index, line := range lines {
		server, ok := codexMCPTableName(strings.TrimSpace(line))
		if ok && server == name {
			start = index
			continue
		}
		if start >= 0 && strings.HasPrefix(strings.TrimSpace(line), "[") {
			end = index
			break
		}
	}
	if start < 0 {
		return fmt.Errorf("Codex MCP table for %q was not created", name)
	}
	encoded, _ := json.Marshal(sortedStrings(variables))
	line := "env_vars = " + string(encoded)
	found := false
	envPattern := regexp.MustCompile(`^\s*env_vars\s*=`)
	for index := start + 1; index < end; index++ {
		if envPattern.MatchString(lines[index]) {
			lines[index] = line
			found = true
			break
		}
	}
	if !found {
		lines = append(lines[:end], append([]string{line}, lines[end:]...)...)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return writeAtomic(path, []byte(strings.Join(lines, "\n")), info.Mode().Perm())
}

func codexMCPTableName(line string) (string, bool) {
	if !strings.HasPrefix(line, "[mcp_servers.") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(line, "[mcp_servers."), "]")
	if strings.HasPrefix(value, `"`) {
		unquoted, err := strconv.Unquote(value)
		return unquoted, err == nil
	}
	return value, true
}
