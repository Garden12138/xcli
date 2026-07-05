package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/tailscale/hujson"
)

// NewProjectManagers returns direct-file managers for the shared configuration
// files loaded by each supported client from a project directory.
func NewProjectManagers(_ config.Config, projectDir string) (map[string]Manager, error) {
	return map[string]Manager{
		"claude": &projectJSONManager{
			target: "claude", path: filepath.Join(projectDir, ".mcp.json"), key: "mcpServers",
		},
		"codex": &projectCodexManager{path: filepath.Join(projectDir, ".codex", "config.toml")},
		"gemini": &projectJSONManager{
			target: "gemini", path: filepath.Join(projectDir, ".gemini", "settings.json"), key: "mcpServers",
		},
		"opencode": &projectJSONManager{
			target: "opencode", path: filepath.Join(projectDir, "opencode.json"), key: "mcp",
		},
	}, nil
}

type projectJSONManager struct {
	target string
	path   string
	key    string
}

func (m *projectJSONManager) Load(context.Context) (map[string]Entry, error) {
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
	switch m.target {
	case "claude":
		return parseJSONEntries(standard, m.key, parseClaudeEntry)
	case "gemini":
		return parseJSONEntries(standard, m.key, parseGeminiEntry)
	case "opencode":
		return parseJSONEntries(standard, m.key, parseOpenCodeEntry)
	default:
		return nil, fmt.Errorf("unsupported project JSON MCP target %q", m.target)
	}
}

func (m *projectJSONManager) Apply(_ context.Context, change Change) error {
	data, err := os.ReadFile(m.path)
	mode := os.FileMode(0o644)
	if errors.Is(err, os.ErrNotExist) {
		data = []byte("{}\n")
	} else if err != nil {
		return err
	} else {
		info, statErr := os.Stat(m.path)
		if statErr != nil {
			return statErr
		}
		mode = info.Mode().Perm()
	}
	document, err := hujson.Parse(data)
	if err != nil {
		return err
	}
	if err := updateProjectJSONDocument(&document, m.key, m.target, change); err != nil {
		return err
	}
	packed := document.Pack()
	if len(packed) == 0 || packed[len(packed)-1] != '\n' {
		packed = append(packed, '\n')
	}
	return writeProjectAtomic(m.path, packed, mode)
}

func updateProjectJSONDocument(document *hujson.Value, key, target string, change Change) error {
	root, ok := document.Value.(*hujson.Object)
	if !ok {
		return fmt.Errorf("%s project configuration root must be an object", target)
	}
	serversValue := document.Find("/" + key)
	if serversValue == nil {
		if change.Action == ActionRemove {
			return fmt.Errorf("%s MCP server %q does not exist", target, change.Server)
		}
		member, err := newObjectMember(key, map[string]interface{}{change.Server: projectJSONValue(target, *change.desired)})
		if err != nil {
			return err
		}
		root.Members = append(root.Members, member)
		return nil
	}
	servers, ok := serversValue.Value.(*hujson.Object)
	if !ok {
		return fmt.Errorf("%s %s configuration must be an object", target, key)
	}
	index := objectMemberIndex(servers, change.Server)
	if change.Action == ActionRemove {
		if index < 0 {
			return fmt.Errorf("%s MCP server %q does not exist", target, change.Server)
		}
		servers.Members = append(servers.Members[:index], servers.Members[index+1:]...)
		return nil
	}
	member, err := newObjectMember(change.Server, projectJSONValue(target, *change.desired))
	if err != nil {
		return err
	}
	if index >= 0 {
		member.Name.BeforeExtra = servers.Members[index].Name.BeforeExtra
		member.Value.AfterExtra = servers.Members[index].Value.AfterExtra
		servers.Members[index] = member
	} else {
		servers.Members = append(servers.Members, member)
	}
	return nil
}

func projectJSONValue(target string, entry Entry) map[string]interface{} {
	switch target {
	case "claude":
		value := map[string]interface{}{"type": entry.Transport}
		if entry.Transport == "http" {
			value["url"] = entry.URL
			return value
		}
		value["command"] = entry.Command
		value["args"] = entry.Args
		if len(entry.EnvVars) > 0 {
			environment := map[string]string{}
			for _, key := range entry.EnvVars {
				environment[key] = "${" + key + "}"
			}
			value["env"] = environment
		}
		return value
	case "gemini":
		if entry.Transport == "http" {
			return map[string]interface{}{"httpUrl": entry.URL}
		}
		value := map[string]interface{}{"command": entry.Command, "args": entry.Args}
		if len(entry.EnvVars) > 0 {
			environment := map[string]string{}
			for _, key := range entry.EnvVars {
				environment[key] = "$" + key
			}
			value["env"] = environment
		}
		return value
	case "opencode":
		return openCodeValue(entry)
	default:
		return map[string]interface{}{}
	}
}

type projectCodexManager struct{ path string }

type codexProjectDocument struct {
	MCPServers map[string]codexProjectEntry `toml:"mcp_servers"`
}

type codexProjectEntry struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	EnvVars []string `toml:"env_vars"`
	URL     string   `toml:"url"`
}

func (m *projectCodexManager) Load(context.Context) (map[string]Entry, error) {
	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	return parseCodexProjectEntries(data)
}

func parseCodexProjectEntries(data []byte) (map[string]Entry, error) {
	var document codexProjectDocument
	if _, err := toml.Decode(string(data), &document); err != nil {
		return nil, err
	}
	result := map[string]Entry{}
	for name, value := range document.MCPServers {
		if value.Command != "" {
			result[name] = Entry{Transport: "stdio", Command: value.Command, Args: value.Args, EnvVars: sortedStrings(value.EnvVars)}
		} else if value.URL != "" {
			result[name] = Entry{Transport: "http", URL: value.URL}
		} else {
			result[name] = Entry{Transport: "unknown"}
		}
	}
	return result, nil
}

func (m *projectCodexManager) Apply(_ context.Context, change Change) error {
	data, err := os.ReadFile(m.path)
	mode := os.FileMode(0o644)
	if errors.Is(err, os.ErrNotExist) {
		data = nil
	} else if err != nil {
		return err
	} else {
		info, statErr := os.Stat(m.path)
		if statErr != nil {
			return statErr
		}
		mode = info.Mode().Perm()
	}
	if _, err := parseCodexProjectEntries(data); err != nil {
		return err
	}
	updated, err := updateCodexProjectDocument(data, change)
	if err != nil {
		return err
	}
	return writeProjectAtomic(m.path, updated, mode)
}

func updateCodexProjectDocument(data []byte, change Change) ([]byte, error) {
	text := string(data)
	hadFinalNewline := strings.HasSuffix(text, "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	start, end := -1, -1
	for index, line := range lines {
		server, exact, ok := codexMCPTableRoot(strings.TrimSpace(line))
		if ok && exact && server == change.Server {
			start = index
			end = codexTableBoundary(lines, index+1, len(lines))
			for next := index + 1; next < len(lines); next++ {
				trimmed := strings.TrimSpace(lines[next])
				if !strings.HasPrefix(trimmed, "[") {
					continue
				}
				nestedServer, _, nested := codexMCPTableRoot(trimmed)
				if !nested || nestedServer != change.Server {
					end = codexTableBoundary(lines, index+1, next)
					break
				}
			}
			break
		}
	}
	if change.Action == ActionRemove {
		if start < 0 {
			return nil, fmt.Errorf("Codex MCP server %q does not exist", change.Server)
		}
		lines = append(lines[:start], lines[end:]...)
		lines = trimExtraBlankLine(lines, start)
	} else {
		block := codexProjectBlock(change.Server, *change.desired)
		if start >= 0 {
			lines = append(append(append([]string{}, lines[:start]...), block...), lines[end:]...)
		} else {
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
				lines = append(lines, "")
			}
			lines = append(lines, block...)
		}
	}
	result := strings.Join(lines, "\n")
	if result != "" && (hadFinalNewline || change.Action != ActionRemove) {
		result += "\n"
	}
	return []byte(result), nil
}

func codexTableBoundary(lines []string, minimum, end int) int {
	for end > minimum {
		trimmed := strings.TrimSpace(lines[end-1])
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			break
		}
		end--
	}
	return end
}

func codexMCPTableRoot(line string) (string, bool, bool) {
	if !strings.HasPrefix(line, "[mcp_servers.") || !strings.HasSuffix(line, "]") || strings.HasPrefix(line, "[[") {
		return "", false, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(line, "[mcp_servers."), "]")
	if strings.HasPrefix(value, `"`) {
		escaped := false
		for index := 1; index < len(value); index++ {
			switch {
			case escaped:
				escaped = false
			case value[index] == '\\':
				escaped = true
			case value[index] == '"':
				name, err := strconv.Unquote(value[:index+1])
				if err != nil {
					return "", false, false
				}
				remainder := value[index+1:]
				return name, remainder == "", remainder == "" || strings.HasPrefix(remainder, ".")
			}
		}
		return "", false, false
	}
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		return value[:dot], false, true
	}
	return value, true, value != ""
}

func codexProjectBlock(name string, entry Entry) []string {
	encodedName, _ := json.Marshal(name)
	lines := []string{"[mcp_servers." + string(encodedName) + "]"}
	if entry.Transport == "http" {
		encodedURL, _ := json.Marshal(entry.URL)
		return append(lines, "url = "+string(encodedURL))
	}
	encodedCommand, _ := json.Marshal(entry.Command)
	encodedArgs, _ := json.Marshal(entry.Args)
	lines = append(lines, "command = "+string(encodedCommand), "args = "+string(encodedArgs))
	if len(entry.EnvVars) > 0 {
		variables := append([]string(nil), entry.EnvVars...)
		sort.Strings(variables)
		encodedVariables, _ := json.Marshal(variables)
		lines = append(lines, "env_vars = "+string(encodedVariables))
	}
	return lines
}

func trimExtraBlankLine(lines []string, around int) []string {
	if around > 0 && around < len(lines) && strings.TrimSpace(lines[around-1]) == "" && strings.TrimSpace(lines[around]) == "" {
		return append(lines[:around], lines[around+1:]...)
	}
	return lines
}

func writeProjectAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".xcli-mcp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
