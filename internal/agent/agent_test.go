package agent

import (
	"math"
	"reflect"
	"strings"
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

func TestBuiltinACPCommands(t *testing.T) {
	cfg := config.Defaults()
	gemini := cfg.Agents["gemini"]
	gemini.Args = []string{"--model", "pro"}
	cfg.Agents["gemini"] = gemini
	opencode := cfg.Agents["opencode"]
	opencode.Args = []string{"--model", "provider/model"}
	cfg.Agents["opencode"] = opencode

	tests := []struct {
		name        string
		wantCommand string
		wantArgs    []string
		wantHint    string
	}{
		{name: "claude", wantCommand: "claude-agent-acp", wantArgs: []string{"--debug"}, wantHint: "@agentclientprotocol/claude-agent-acp"},
		{name: "codex", wantCommand: "codex-acp", wantArgs: []string{"--debug"}, wantHint: "@agentclientprotocol/codex-acp"},
		{name: "gemini", wantCommand: "gemini", wantArgs: []string{"--model", "pro", "--acp", "--debug"}},
		{name: "opencode", wantCommand: "opencode", wantArgs: []string{"--model", "provider/model", "acp", "--debug"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := Definition{Name: test.name, Config: cfg.Agents[test.name]}
			launch, err := definition.ACP([]string{"--debug"})
			if err != nil {
				t.Fatal(err)
			}
			if launch.Command != test.wantCommand || !reflect.DeepEqual(launch.Args, test.wantArgs) {
				t.Fatalf("launch = %#v, want command %q args %#v", launch, test.wantCommand, test.wantArgs)
			}
			if test.wantHint != "" && !strings.Contains(launch.InstallHint, test.wantHint) {
				t.Fatalf("install hint = %q, want package %q", launch.InstallHint, test.wantHint)
			}
		})
	}
}

func TestACPOverrideIsComplete(t *testing.T) {
	definition := Definition{Name: "custom", Config: config.AgentConfig{
		Adapter: "gemini", Command: "gemini", Args: []string{"--model", "pro"},
		ACP: &config.ACPConfig{Command: "custom-acp", Args: []string{"--stdio"}},
	}}
	launch, err := definition.ACP([]string{"--debug"})
	if err != nil {
		t.Fatal(err)
	}
	if launch.Command != "custom-acp" || !reflect.DeepEqual(launch.Args, []string{"--stdio", "--debug"}) {
		t.Fatalf("unexpected override launch: %#v", launch)
	}
	if launch.InstallHint != "" {
		t.Fatalf("custom override inherited install hint: %#v", launch)
	}
}

func TestGenericACPRequiresConfiguration(t *testing.T) {
	definition := Definition{Name: "custom", Config: config.AgentConfig{Adapter: "generic", Command: "custom"}}
	_, err := definition.ACP(nil)
	if err == nil || !strings.Contains(err.Error(), "configure agents.custom.acp") {
		t.Fatalf("expected unsupported ACP error, got %v", err)
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

func TestParseCodexUsage(t *testing.T) {
	data := []byte("{\"type\":\"thread.started\",\"thread_id\":\"thread-1\"}\n" +
		"{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"done\"}}\n" +
		"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1000,\"cached_input_tokens\":800,\"output_tokens\":100,\"reasoning_output_tokens\":40,\"total_tokens\":1100}}\n")
	result := ParseStructured("codex", "jsonl", data)
	want := Usage{InputTokens: 200, CacheReadTokens: 800, OutputTokens: 60, ReasoningTokens: 40, TotalTokens: 1100}
	assertUsage(t, result.Usage, want)
}

func TestParseClaudeUsageAndZeroCost(t *testing.T) {
	data := []byte("{\"type\":\"result\",\"result\":\"done\",\"usage\":{\"input_tokens\":100,\"cache_read_input_tokens\":200,\"cache_creation_input_tokens\":50,\"output_tokens\":30},\"total_cost_usd\":0}\n")
	result := ParseStructured("claude", "jsonl", data)
	want := Usage{InputTokens: 100, CacheReadTokens: 200, CacheWriteTokens: 50, OutputTokens: 30, TotalTokens: 380}
	assertUsage(t, result.Usage, want)
	if result.Usage.EstimatedCostUSD == nil || *result.Usage.EstimatedCostUSD != 0 {
		t.Fatalf("zero estimated cost was not preserved: %#v", result.Usage)
	}
}

func TestParseGeminiUsage(t *testing.T) {
	data := []byte("{\"type\":\"result\",\"status\":\"success\",\"stats\":{\"total_tokens\":1000,\"input_tokens\":900,\"output_tokens\":50,\"cached\":600,\"input\":300}}\n")
	result := ParseStructured("gemini", "jsonl", data)
	want := Usage{InputTokens: 300, CacheReadTokens: 600, OutputTokens: 50, ReasoningTokens: 50, TotalTokens: 1000}
	assertUsage(t, result.Usage, want)
}

func TestParseOpenCodeUsageDeduplicatesParts(t *testing.T) {
	first := "{\"type\":\"step_finish\",\"part\":{\"id\":\"part-1\",\"cost\":0.1,\"tokens\":{\"input\":100,\"output\":10,\"reasoning\":0,\"total\":165,\"cache\":{\"read\":50,\"write\":5}}}}\n"
	second := "{\"type\":\"step_finish\",\"part\":{\"id\":\"part-2\",\"cost\":0.2,\"tokens\":{\"input\":20,\"output\":5,\"reasoning\":3,\"total\":28,\"cache\":{\"read\":0,\"write\":0}}}}\n"
	data := []byte(first + first + second)
	result := ParseStructured("opencode", "jsonl", data)
	want := Usage{InputTokens: 120, CacheReadTokens: 50, CacheWriteTokens: 5, OutputTokens: 15, ReasoningTokens: 3, TotalTokens: 193}
	assertUsage(t, result.Usage, want)
	if result.Usage.EstimatedCostUSD == nil || math.Abs(*result.Usage.EstimatedCostUSD-0.3) > 1e-12 {
		t.Fatalf("unexpected estimated cost: %#v", result.Usage)
	}
}

func TestParseUsageIgnoresMalformedNumbers(t *testing.T) {
	data := []byte("{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":-1,\"cached_input_tokens\":1.5,\"output_tokens\":\"3\"}}\n")
	result := ParseStructured("codex", "jsonl", data)
	if result.Usage != nil {
		t.Fatalf("malformed usage should be ignored: %#v", result.Usage)
	}
}

func TestParseGenericOutputDoesNotInferUsage(t *testing.T) {
	data := []byte(`{"output":"done","usage":{"input_tokens":10}}`)
	result := ParseStructured("generic", "json", data)
	if result.Usage != nil {
		t.Fatalf("generic usage should be unsupported: %#v", result.Usage)
	}
}

func assertUsage(t *testing.T, got *Usage, want Usage) {
	t.Helper()
	if got == nil {
		t.Fatalf("usage is nil, want %#v", want)
	}
	gotWithoutCost := *got
	gotWithoutCost.EstimatedCostUSD = nil
	if !reflect.DeepEqual(gotWithoutCost, want) {
		t.Fatalf("usage = %#v, want %#v", gotWithoutCost, want)
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
