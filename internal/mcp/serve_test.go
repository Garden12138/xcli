package mcp

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
)

func TestBuildServeSpecUsesMinimalEnvironmentAndConfigRelativeCwd(t *testing.T) {
	directory := t.TempDir()
	work := filepath.Join(directory, "tools")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{
		"local": {
			Transport: "stdio", Command: "server", Args: []string{"--stdio"}, Cwd: "tools",
			Env: map[string]string{"LOG_LEVEL": "debug"}, EnvVars: []string{"SERVICE_TOKEN"},
		},
	}
	parent := []string{
		"PATH=/bin", "HOME=/home/test", "LANG=en_US.UTF-8", "LC_ALL=C",
		"SERVICE_TOKEN=secret", "OTHER_SECRET=hidden", "LOG_LEVEL=parent",
	}
	spec, err := BuildServeSpec(cfg, filepath.Join(directory, "config.yaml"), "local", parent)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command.Command != "server" || !reflect.DeepEqual(spec.Command.Args, []string{"--stdio"}) {
		t.Fatalf("unexpected command: %#v", spec.Command)
	}
	if spec.Dir != work {
		t.Fatalf("cwd = %q, want %q", spec.Dir, work)
	}
	environment := strings.Join(spec.Env, "\n")
	for _, expected := range []string{"HOME=/home/test", "LANG=en_US.UTF-8", "LC_ALL=C", "LOG_LEVEL=debug", "PATH=/bin", "SERVICE_TOKEN=secret"} {
		if !strings.Contains(environment, expected) {
			t.Fatalf("environment %q missing %q", environment, expected)
		}
	}
	if strings.Contains(environment, "OTHER_SECRET") {
		t.Fatalf("unlisted secret leaked into environment: %q", environment)
	}
}

func TestBuildServeSpecRejectsMissingEnvironmentAndHTTP(t *testing.T) {
	cfg := config.Defaults()
	cfg.MCP.Servers = map[string]config.MCPServer{
		"local":  {Transport: "stdio", Command: "server", EnvVars: []string{"TOKEN"}},
		"remote": {Transport: "http", URL: "https://example.com/mcp"},
	}
	if _, err := BuildServeSpec(cfg, "/tmp/config.yaml", "local", []string{"PATH=/bin"}); err == nil || !strings.Contains(err.Error(), "requires environment variable TOKEN") {
		t.Fatalf("expected missing environment error, got %v", err)
	}
	if _, err := BuildServeSpec(cfg, "/tmp/config.yaml", "remote", nil); err == nil || !strings.Contains(err.Error(), "only stdio") {
		t.Fatalf("expected HTTP serve error, got %v", err)
	}
	if _, err := BuildServeSpec(cfg, "/tmp/config.yaml", "missing", nil); err == nil || !strings.Contains(err.Error(), "unknown MCP server") {
		t.Fatalf("expected unknown server error, got %v", err)
	}
}
