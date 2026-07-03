package agent

import (
	"reflect"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
)

func TestBuiltinRunCommands(t *testing.T) {
	cfg := config.Defaults()
	tests := []struct {
		name string
		want []string
	}{
		{"claude", []string{"--model", "opus", "-p", "fix it", "--output-format", "stream-json", "--verbose"}},
		{"codex", []string{"exec", "--json", "--sandbox", "workspace-write", "--", "fix it"}},
		{"gemini", []string{"--model", "pro", "-p", "fix it", "--output-format", "stream-json"}},
		{"opencode", []string{"run", "--format", "json", "--model", "provider/model", "fix it"}},
	}
	extra := map[string][]string{
		"claude":   {"--model", "opus"},
		"codex":    {"--sandbox", "workspace-write"},
		"gemini":   {"--model", "pro"},
		"opencode": {"--model", "provider/model"},
	}
	for _, test := range tests {
		definition := Definition{Name: test.name, Config: cfg.Agents[test.name]}
		spec, err := definition.Run("fix it", true, extra[test.name])
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(spec.Args, test.want) {
			t.Errorf("%s args = %#v, want %#v", test.name, spec.Args, test.want)
		}
	}
}

func TestGenericPromptTemplateIsAnArgumentNotShell(t *testing.T) {
	definition := Definition{Name: "custom", Config: config.AgentConfig{
		Adapter: "generic", Command: "fake", RunArgs: []string{"run", "{{ prompt }}"},
	}}
	spec, err := definition.Run("hello; touch /tmp/never", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Args) != 2 || spec.Args[1] != "hello; touch /tmp/never" {
		t.Fatalf("prompt was not preserved as one argv entry: %#v", spec.Args)
	}
}

func TestParseStructuredResults(t *testing.T) {
	data := []byte("{\"type\":\"thread.started\",\"thread_id\":\"thread-1\"}\n" +
		"{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"done\"}}\n")
	result := ParseStructured("codex", "jsonl", data)
	if result.SessionID != "thread-1" || result.Output != "done" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestParseGenericPrettyJSON(t *testing.T) {
	data := []byte("{\n  \"session_id\": \"session-1\",\n  \"output\": \"done\"\n}\n")
	result := ParseStructured("generic", "json", data)
	if result.SessionID != "session-1" || result.Output != "done" {
		t.Fatalf("unexpected generic JSON result: %#v", result)
	}
}

func TestParseGeminiStreamingMessages(t *testing.T) {
	data := []byte("{\"type\":\"init\",\"session_id\":\"gemini-1\"}\n" +
		"{\"type\":\"message\",\"role\":\"assistant\",\"content\":\"first \"}\n" +
		"{\"type\":\"message\",\"role\":\"assistant\",\"content\":\"second\"}\n")
	result := ParseStructured("gemini", "jsonl", data)
	if result.SessionID != "gemini-1" || result.Output != "first second" {
		t.Fatalf("unexpected Gemini result: %#v", result)
	}
}

func TestInstallTableContainsExpectedPackages(t *testing.T) {
	if got := installMethods("claude")["npm"].Args[2]; got != "@anthropic-ai/claude-code" {
		t.Fatalf("unexpected Claude package %q", got)
	}
	if got := installMethods("opencode")["brew"].Args[1]; got != "anomalyco/tap/opencode" {
		t.Fatalf("unexpected OpenCode formula %q", got)
	}
}

func TestVersionLineUsesFinalNonEmptyLine(t *testing.T) {
	got := versionLine("WARNING: setup failed\n\ncodex-cli 0.142.5\n")
	if got != "codex-cli 0.142.5" {
		t.Fatalf("version line = %q", got)
	}
}
