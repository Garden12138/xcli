package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
)

var baseEnvironment = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
	"TMPDIR": true, "TMP": true, "TEMP": true, "LANG": true,
}

type ServeSpec struct {
	Command agent.CommandSpec
	Dir     string
	Env     []string
}

func BuildServeSpec(cfg config.Config, configPath, name string, parent []string) (ServeSpec, error) {
	server, ok := cfg.MCP.Servers[name]
	if !ok {
		return ServeSpec{}, fmt.Errorf("unknown MCP server %q", name)
	}
	if server.Transport != "stdio" {
		return ServeSpec{}, fmt.Errorf("MCP server %q uses %s transport; only stdio servers can be served", name, server.Transport)
	}
	parentValues := environmentMap(parent)
	values := map[string]string{}
	for key, value := range parentValues {
		if baseEnvironment[key] || strings.HasPrefix(key, "LC_") {
			values[key] = value
		}
	}
	for _, key := range server.EnvVars {
		value, ok := parentValues[key]
		if !ok {
			return ServeSpec{}, fmt.Errorf("MCP server %q requires environment variable %s", name, key)
		}
		values[key] = value
	}
	for key, value := range server.Env {
		values[key] = value
	}
	directory, err := serveDirectory(configPath, server.Cwd)
	if err != nil {
		return ServeSpec{}, fmt.Errorf("MCP server %q cwd: %w", name, err)
	}
	return ServeSpec{
		Command: agent.CommandSpec{Command: server.Command, Args: append([]string(nil), server.Args...)},
		Dir:     directory,
		Env:     environmentList(values),
	}, nil
}

// ResolveProjectConfig walks from start toward the filesystem root looking for
// a project-relative xcli configuration. A symlink may point within a candidate
// project root, but it may not escape that root.
func ResolveProjectConfig(start, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("project configuration path %q must be relative", relative)
	}
	relative = filepath.Clean(filepath.FromSlash(relative))
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("project configuration path %q escapes the project", relative)
	}
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(current); resolveErr == nil {
		current = resolved
	}
	for {
		candidate := filepath.Join(current, relative)
		info, statErr := os.Stat(candidate)
		if statErr == nil {
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("project configuration %s is not a regular file", candidate)
			}
			resolved, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				return "", err
			}
			inside, err := filepath.Rel(current, resolved)
			if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("project configuration symlink %s escapes project %s", candidate, current)
			}
			return resolved, nil
		}
		if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf("project configuration %q was not found from %s or its parents", filepath.ToSlash(relative), start)
}

func serveDirectory(configPath, cwd string) (string, error) {
	if cwd == "" {
		return os.Getwd()
	}
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(filepath.Dir(configPath), cwd)
	}
	absolute, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", absolute)
	}
	return absolute, nil
}

func environmentMap(entries []string) map[string]string {
	values := map[string]string{}
	for _, entry := range entries {
		if index := strings.IndexByte(entry, '='); index >= 0 {
			values[entry[:index]] = entry[index+1:]
		}
	}
	return values
}

func environmentList(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}
