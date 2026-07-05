package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Garden12138/xcli/internal/config"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/tailscale/hujson"
)

type ImportReadOptions struct {
	Scope        string
	ProjectDir   string
	SourceConfig string
}

var importMCPNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)

var (
	geminiDollarExpansion  = regexp.MustCompile(`\$[A-Za-z_]`)
	geminiPercentExpansion = regexp.MustCompile(`%[A-Za-z_][A-Za-z0-9_]*%`)
)

func NativeConfigPaths(cfg config.Config, options ImportReadOptions) (map[string]string, error) {
	if options.Scope == ScopeProject {
		return map[string]string{
			"claude":   filepath.Join(options.ProjectDir, ".mcp.json"),
			"codex":    filepath.Join(options.ProjectDir, ".codex", "config.toml"),
			"gemini":   filepath.Join(options.ProjectDir, ".gemini", "settings.json"),
			"opencode": filepath.Join(options.ProjectDir, "opencode.json"),
		}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	codexHome := filepath.Join(home, ".codex")
	if agentConfig, ok := cfg.Agents["codex"]; ok {
		environment, envErr := xruntime.BuildEnvironment(os.Environ(), cfg, agentConfig, "")
		if envErr != nil {
			return nil, envErr
		}
		if value := environmentMap(environment)["CODEX_HOME"]; value != "" {
			codexHome = value
		}
	}
	openCodeRoot := os.Getenv("XDG_CONFIG_HOME")
	if openCodeRoot == "" {
		openCodeRoot = filepath.Join(home, ".config")
	}
	return map[string]string{
		"claude":   filepath.Join(home, ".claude.json"),
		"codex":    filepath.Join(codexHome, "config.toml"),
		"gemini":   filepath.Join(home, ".gemini", "settings.json"),
		"opencode": filepath.Join(openCodeRoot, "opencode", "opencode.json"),
	}, nil
}

func ReadNativeSnapshot(target, path string, options ImportReadOptions) (NativeSnapshot, error) {
	snapshot := NativeSnapshot{Target: target, Path: path, Entries: map[string]NativeCandidate{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	snapshot.Exists = true
	snapshot.Fingerprint = contentFingerprint(data)
	var entries map[string]NativeCandidate
	switch target {
	case "claude":
		entries, err = readJSONNativeEntries(data, "mcpServers", target, options)
	case "gemini":
		entries, err = readJSONNativeEntries(data, "mcpServers", target, options)
	case "opencode":
		entries, err = readJSONNativeEntries(data, "mcp", target, options)
	case "codex":
		entries, err = readCodexNativeEntries(data, options)
	default:
		return snapshot, fmt.Errorf("unknown MCP import target %q", target)
	}
	if err != nil {
		return snapshot, err
	}
	for name, candidate := range entries {
		candidate.Name = name
		candidate.Target = target
		if (!importMCPNamePattern.MatchString(name) || name == "workspace") && candidate.Unsupported == "" {
			candidate.Unsupported = "server name is not portable across supported agents"
		}
		if candidate.Unsupported == "" {
			candidate.Wrapper = recognizeWrapper(candidate.Fingerprint, options)
		}
		snapshot.Entries[name] = candidate
	}
	return snapshot, nil
}

func readJSONNativeEntries(data []byte, key, target string, options ImportReadOptions) (map[string]NativeCandidate, error) {
	standard, err := hujson.Standardize(data)
	if err != nil {
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(standard, &root); err != nil {
		return nil, err
	}
	servers := map[string]json.RawMessage{}
	if raw := root[key]; raw != nil {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, fmt.Errorf("%s must be an object", key)
		}
	}
	result := map[string]NativeCandidate{}
	for name, raw := range servers {
		candidate := NativeCandidate{}
		switch target {
		case "claude":
			candidate = inspectClaudeNative(raw, options)
		case "gemini":
			candidate = inspectGeminiNative(raw, options)
		case "opencode":
			candidate = inspectOpenCodeNative(raw, options)
		}
		result[name] = candidate
	}
	return result, nil
}

func inspectClaudeNative(raw json.RawMessage, options ImportReadOptions) NativeCandidate {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return unsupportedCandidate("server definition is malformed")
	}
	if hasUnknownJSONFields(fields, "type", "command", "args", "env", "cwd", "url") {
		return unsupportedCandidate("contains vendor-specific or advanced fields")
	}
	var value struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		Cwd     string            `json:"cwd"`
		URL     string            `json:"url"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return unsupportedCandidate("server definition has invalid field types")
	}
	if value.Command != "" && value.URL != "" {
		return unsupportedCandidate("server defines more than one transport")
	}
	if value.Command != "" || value.Type == "stdio" || value.Type == "" && value.URL == "" {
		if value.Type != "" && value.Type != "stdio" {
			return unsupportedCandidate("server transport conflicts with stdio fields")
		}
		if value.Command == "" || containsExpansion(append([]string{value.Command}, value.Args...), "claude") {
			return unsupportedCandidate("stdio command or arguments use unsupported variable expansion")
		}
		envVars, ok := exactEnvironmentReferences(value.Env, "claude")
		if !ok {
			return unsupportedCandidate("contains static or remapped environment values")
		}
		cwd, reason := importCwd(value.Cwd, options)
		if reason != "" {
			return unsupportedCandidate(reason)
		}
		server := config.MCPServer{Transport: "stdio", Command: value.Command, Args: value.Args, Cwd: cwd, EnvVars: envVars}
		return supportedCandidate(server, Entry{Transport: "stdio", Command: value.Command, Args: value.Args, Cwd: value.Cwd, EnvVars: envVars})
	}
	if (value.Type == "http" || value.Type == "streamable-http") && value.URL != "" {
		if len(value.Args) > 0 || len(value.Env) > 0 || value.Cwd != "" {
			return unsupportedCandidate("HTTP server contains stdio-only fields")
		}
		if containsExpansion([]string{value.URL}, "claude") {
			return unsupportedCandidate("HTTP URL uses unsupported variable expansion")
		}
		if !validImportHTTPURL(value.URL) {
			return unsupportedCandidate("HTTP URL is invalid")
		}
		return supportedCandidate(config.MCPServer{Transport: "http", URL: value.URL}, Entry{Transport: "http", URL: value.URL})
	}
	return unsupportedCandidate("transport is not supported")
}

func inspectGeminiNative(raw json.RawMessage, options ImportReadOptions) NativeCandidate {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return unsupportedCandidate("server definition is malformed")
	}
	if hasUnknownJSONFields(fields, "command", "args", "env", "cwd", "httpUrl") {
		return unsupportedCandidate("contains vendor-specific or advanced fields")
	}
	var value struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		Cwd     string            `json:"cwd"`
		HTTPURL string            `json:"httpUrl"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return unsupportedCandidate("server definition has invalid field types")
	}
	if value.Command != "" && value.HTTPURL != "" {
		return unsupportedCandidate("server defines more than one transport")
	}
	if value.Command != "" {
		if containsExpansion(append([]string{value.Command}, value.Args...), "gemini") {
			return unsupportedCandidate("stdio command or arguments use unsupported variable expansion")
		}
		envVars, ok := exactEnvironmentReferences(value.Env, "gemini")
		if !ok {
			return unsupportedCandidate("contains static or remapped environment values")
		}
		cwd, reason := importCwd(value.Cwd, options)
		if reason != "" {
			return unsupportedCandidate(reason)
		}
		server := config.MCPServer{Transport: "stdio", Command: value.Command, Args: value.Args, Cwd: cwd, EnvVars: envVars}
		return supportedCandidate(server, Entry{Transport: "stdio", Command: value.Command, Args: value.Args, Cwd: value.Cwd, EnvVars: envVars})
	}
	if value.HTTPURL != "" {
		if len(value.Args) > 0 || len(value.Env) > 0 || value.Cwd != "" {
			return unsupportedCandidate("HTTP server contains stdio-only fields")
		}
		if containsExpansion([]string{value.HTTPURL}, "gemini") {
			return unsupportedCandidate("HTTP URL uses unsupported variable expansion")
		}
		if !validImportHTTPURL(value.HTTPURL) {
			return unsupportedCandidate("HTTP URL is invalid")
		}
		return supportedCandidate(config.MCPServer{Transport: "http", URL: value.HTTPURL}, Entry{Transport: "http", URL: value.HTTPURL})
	}
	return unsupportedCandidate("transport is not supported")
}

func inspectOpenCodeNative(raw json.RawMessage, options ImportReadOptions) NativeCandidate {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return unsupportedCandidate("server definition is malformed")
	}
	if hasUnknownJSONFields(fields, "type", "command", "environment", "cwd", "url", "enabled") {
		return unsupportedCandidate("contains vendor-specific or advanced fields")
	}
	var value struct {
		Type        string            `json:"type"`
		Command     []string          `json:"command"`
		Environment map[string]string `json:"environment"`
		Cwd         string            `json:"cwd"`
		URL         string            `json:"url"`
		Enabled     *bool             `json:"enabled"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return unsupportedCandidate("server definition has invalid field types")
	}
	if len(value.Command) > 0 && value.URL != "" {
		return unsupportedCandidate("server defines more than one transport")
	}
	if value.Enabled != nil && !*value.Enabled {
		return unsupportedCandidate("disabled servers cannot be represented without changing behavior")
	}
	if value.Type == "local" && len(value.Command) > 0 {
		if containsExpansion(value.Command, "opencode") {
			return unsupportedCandidate("stdio command or arguments use unsupported variable expansion")
		}
		envVars, ok := exactEnvironmentReferences(value.Environment, "opencode")
		if !ok {
			return unsupportedCandidate("contains static or remapped environment values")
		}
		cwd, reason := importCwd(value.Cwd, options)
		if reason != "" {
			return unsupportedCandidate(reason)
		}
		server := config.MCPServer{Transport: "stdio", Command: value.Command[0], Args: value.Command[1:], Cwd: cwd, EnvVars: envVars}
		return supportedCandidate(server, Entry{Transport: "stdio", Command: value.Command[0], Args: value.Command[1:], Cwd: value.Cwd, EnvVars: envVars})
	}
	if value.Type == "remote" && value.URL != "" {
		if len(value.Environment) > 0 || value.Cwd != "" {
			return unsupportedCandidate("HTTP server contains stdio-only fields")
		}
		if containsExpansion([]string{value.URL}, "opencode") {
			return unsupportedCandidate("HTTP URL uses unsupported variable expansion")
		}
		if !validImportHTTPURL(value.URL) {
			return unsupportedCandidate("HTTP URL is invalid")
		}
		return supportedCandidate(config.MCPServer{Transport: "http", URL: value.URL}, Entry{Transport: "http", URL: value.URL})
	}
	return unsupportedCandidate("transport is not supported")
}

func readCodexNativeEntries(data []byte, options ImportReadOptions) (map[string]NativeCandidate, error) {
	var root map[string]interface{}
	if _, err := toml.Decode(string(data), &root); err != nil {
		return nil, err
	}
	servers, _ := stringMap(root["mcp_servers"])
	result := map[string]NativeCandidate{}
	for name, raw := range servers {
		fields, ok := stringMap(raw)
		if !ok {
			result[name] = unsupportedCandidate("server definition is malformed")
			continue
		}
		if hasUnknownMapFields(fields, "command", "args", "env", "env_vars", "cwd", "url", "enabled") {
			result[name] = unsupportedCandidate("contains vendor-specific or advanced fields")
			continue
		}
		if rawEnabled, exists := fields["enabled"]; exists {
			enabled, valid := rawEnabled.(bool)
			if !valid {
				result[name] = unsupportedCandidate("enabled has an invalid field type")
				continue
			}
			if !enabled {
				result[name] = unsupportedCandidate("disabled servers cannot be represented without changing behavior")
				continue
			}
		}
		command, commandOK := fields["command"].(string)
		if fields["command"] != nil && !commandOK {
			result[name] = unsupportedCandidate("command has an invalid field type")
			continue
		}
		url, urlOK := fields["url"].(string)
		if fields["url"] != nil && !urlOK {
			result[name] = unsupportedCandidate("url has an invalid field type")
			continue
		}
		if command != "" && url != "" {
			result[name] = unsupportedCandidate("server defines more than one transport")
			continue
		}
		if command != "" {
			args, validArgs := stringSlice(fields["args"])
			if fields["args"] != nil && !validArgs {
				result[name] = unsupportedCandidate("stdio arguments have invalid field types")
				continue
			}
			if fields["env"] != nil {
				env, valid := stringMap(fields["env"])
				if !valid {
					result[name] = unsupportedCandidate("env has an invalid field type")
					continue
				}
				if len(env) > 0 {
					result[name] = unsupportedCandidate("contains static environment values")
					continue
				}
			}
			envVars, validEnv := stringSlice(fields["env_vars"])
			if fields["env_vars"] != nil && !validEnv {
				result[name] = unsupportedCandidate("contains remote or structured environment forwarding")
				continue
			}
			cwd, cwdOK := fields["cwd"].(string)
			if fields["cwd"] != nil && !cwdOK {
				result[name] = unsupportedCandidate("cwd has an invalid field type")
				continue
			}
			importedCwd, reason := importCwd(cwd, options)
			if reason != "" {
				result[name] = unsupportedCandidate(reason)
				continue
			}
			sort.Strings(envVars)
			server := config.MCPServer{Transport: "stdio", Command: command, Args: args, Cwd: importedCwd, EnvVars: envVars}
			result[name] = supportedCandidate(server, Entry{Transport: "stdio", Command: command, Args: args, Cwd: cwd, EnvVars: envVars})
			continue
		}
		if url != "" {
			for _, field := range []string{"args", "env", "env_vars", "cwd"} {
				if value, exists := fields[field]; exists && !isEmptyNativeValue(value) {
					result[name] = unsupportedCandidate("HTTP server contains stdio-only fields")
					url = ""
					break
				}
			}
			if url == "" {
				continue
			}
			if !validImportHTTPURL(url) {
				result[name] = unsupportedCandidate("HTTP URL is invalid")
				continue
			}
			result[name] = supportedCandidate(config.MCPServer{Transport: "http", URL: url}, Entry{Transport: "http", URL: url})
			continue
		}
		result[name] = unsupportedCandidate("transport is not supported")
	}
	return result, nil
}

func supportedCandidate(server config.MCPServer, fingerprint Entry) NativeCandidate {
	server.Args = append([]string(nil), server.Args...)
	server.EnvVars = sortedStrings(server.EnvVars)
	fingerprint.Args = append([]string(nil), fingerprint.Args...)
	fingerprint.EnvVars = sortedStrings(fingerprint.EnvVars)
	return NativeCandidate{Server: server, Fingerprint: fingerprint}
}

func unsupportedCandidate(reason string) NativeCandidate {
	return NativeCandidate{Unsupported: reason}
}

func hasUnknownJSONFields(fields map[string]json.RawMessage, allowed ...string) bool {
	set := map[string]bool{}
	for _, name := range allowed {
		set[name] = true
	}
	for name := range fields {
		if !set[name] {
			return true
		}
	}
	return false
}

func hasUnknownMapFields(fields map[string]interface{}, allowed ...string) bool {
	set := map[string]bool{}
	for _, name := range allowed {
		set[name] = true
	}
	for name := range fields {
		if !set[name] {
			return true
		}
	}
	return false
}

func exactEnvironmentReferences(environment map[string]string, target string) ([]string, bool) {
	result := make([]string, 0, len(environment))
	for key, value := range environment {
		valid := false
		switch target {
		case "claude":
			valid = value == "${"+key+"}"
		case "gemini":
			valid = value == "$"+key || value == "${"+key+"}" || value == "%"+key+"%"
		case "opencode":
			valid = value == "{env:"+key+"}"
		}
		if !valid {
			return nil, false
		}
		result = append(result, key)
	}
	sort.Strings(result)
	return result, true
}

func containsExpansion(values []string, target string) bool {
	for _, value := range values {
		switch target {
		case "claude":
			if strings.Contains(value, "${") {
				return true
			}
		case "gemini":
			if strings.Contains(value, "${") || geminiDollarExpansion.MatchString(value) || geminiPercentExpansion.MatchString(value) {
				return true
			}
		case "opencode":
			if strings.Contains(value, "{env:") {
				return true
			}
		}
	}
	return false
}

func importCwd(value string, options ImportReadOptions) (string, string) {
	if value == "" {
		return "", ""
	}
	if options.Scope == ScopeUser {
		if !filepath.IsAbs(value) {
			return "", "relative cwd in user configuration has invocation-dependent semantics"
		}
		return filepath.Clean(value), ""
	}
	if filepath.IsAbs(value) {
		return "", "absolute cwd is not portable in project configuration"
	}
	absolute := filepath.Clean(filepath.Join(options.ProjectDir, filepath.FromSlash(value)))
	relativeProject, err := filepath.Rel(options.ProjectDir, absolute)
	if err != nil || relativeProject == ".." || strings.HasPrefix(relativeProject, ".."+string(filepath.Separator)) {
		return "", "cwd escapes the project directory"
	}
	relativeSource, err := filepath.Rel(filepath.Dir(options.SourceConfig), absolute)
	if err != nil {
		return "", "cwd cannot be represented relative to the xcli source configuration"
	}
	return filepath.ToSlash(relativeSource), ""
}

func recognizeWrapper(entry Entry, options ImportReadOptions) *WrapperReference {
	args := entry.Args
	if len(args) == 5 && args[0] == "--config" && args[2] == "mcp" && args[3] == "serve" {
		source := canonicalBestEffort(args[1])
		return &WrapperReference{SourceConfig: source, Server: args[4]}
	}
	if len(args) == 5 && args[0] == "mcp" && args[1] == "serve" && args[2] == "--project-config" {
		if options.ProjectDir == "" || filepath.IsAbs(args[3]) {
			return &WrapperReference{Server: args[4]}
		}
		source := canonicalBestEffort(filepath.Join(options.ProjectDir, filepath.FromSlash(args[3])))
		return &WrapperReference{SourceConfig: source, Server: args[4]}
	}
	return nil
}

func canonicalBestEffort(value string) string {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return filepath.Clean(value)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		return resolved
	}
	return filepath.Clean(absolute)
}

func stringMap(value interface{}) (map[string]interface{}, bool) {
	if value == nil {
		return nil, false
	}
	if result, ok := value.(map[string]interface{}); ok {
		return result, true
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Map {
		return nil, false
	}
	result := map[string]interface{}{}
	for _, key := range reflected.MapKeys() {
		if key.Kind() != reflect.String {
			return nil, false
		}
		result[key.String()] = reflected.MapIndex(key).Interface()
	}
	return result, true
}

func stringSlice(value interface{}) ([]string, bool) {
	if value == nil {
		return nil, true
	}
	if values, ok := value.([]string); ok {
		return append([]string(nil), values...), true
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return nil, false
	}
	result := make([]string, reflected.Len())
	for index := 0; index < reflected.Len(); index++ {
		value := reflected.Index(index)
		for value.Kind() == reflect.Interface || value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return nil, false
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.String {
			return nil, false
		}
		result[index] = value.String()
	}
	return result, true
}

func isEmptyNativeValue(value interface{}) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Ptr {
		if reflected.IsNil() {
			return true
		}
		reflected = reflected.Elem()
	}
	switch reflected.Kind() {
	case reflect.String, reflect.Array, reflect.Slice, reflect.Map:
		return reflected.Len() == 0
	}
	return false
}

func validImportHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func contentFingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
