//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

func mcpDrift(f *fixture) (bool, bool, string) {
	servers := map[string]any{"docs": map[string]any{"transport": "http", "url": "https://example.invalid/mcp", "targets": []string{"claude"}}}
	f.configure("cwd", servers)
	args := []string{"--scope", "project", "--project", f.repo, "--target", "claude", "--json"}
	o, _ := f.run(f.repo, append([]string{"mcp", "sync", "--yes"}, args...)...)
	require(o.ExitCode == 0, "initial MCP sync failed")
	path := filepath.Join(f.repo, ".mcp.json")
	native := readObject(path)
	entries := native["mcpServers"].(map[string]any)
	entry := entries["docs"].(map[string]any)
	entry["headers"] = map[string]any{"X-Audit-Change": "added-after-sync"}
	jsonFile(path, native)
	p, plan := f.run(f.repo, append([]string{"mcp", "plan"}, args...)...)
	action := ""
	if changes, ok := plan["changes"].([]any); ok && len(changes) > 0 {
		action, _ = changes[0].(map[string]any)["action"].(string)
	}
	f.configure("cwd", map[string]any{})
	d, _ := f.run(f.repo, append([]string{"mcp", "sync", "--yes"}, args...)...)
	after := readObject(path)["mcpServers"].(map[string]any)
	_, present := after["docs"]
	return p.ExitCode == 0 && action == "noop" && d.ExitCode == 0 && !present,
		action == "conflict" && d.ExitCode != 0 && reflect.DeepEqual(after["docs"], entry),
		fmt.Sprintf("plan_action=%s; delete_exit=%d; externally_edited_entry_preserved=%t", action, d.ExitCode, present)
}

func importEnv(f *fixture) (bool, bool, string) {
	server := map[string]any{"transport": "stdio", "command": "phase0-server", "args": []string{"--fixture"}, "cwd": "..", "env": map[string]string{"LOG_LEVEL": "info"}, "env_vars": []string{"PHASE0_TOKEN"}, "targets": []string{"claude"}}
	f.configure("cwd", map[string]any{"tools": server})
	jsonFile(filepath.Join(f.repo, ".gemini", "settings.json"), map[string]any{"mcpServers": map[string]any{"tools": map[string]any{"command": "xcli", "args": []string{"mcp", "serve", "--project-config", ".xcli/config.yaml", "tools"}}}})
	args := []string{"mcp", "import", "apply", "--scope", "project", "--project", f.repo, "--target", "gemini", "--yes", "--json"}
	o, _ := f.run(f.repo, args...)
	require(o.ExitCode == 0, "MCP import failed")
	data, err := os.ReadFile(f.config)
	must(err)
	var cfg map[string]any
	must(yaml.Unmarshal(data, &cfg))
	actual := cfg["mcp"].(map[string]any)["servers"].(map[string]any)["tools"].(map[string]any)
	expectedData, err := yaml.Marshal(server)
	must(err)
	var expected map[string]any
	must(yaml.Unmarshal(expectedData, &expected))
	expected["targets"] = []any{"claude", "gemini"}
	coverage := reflect.DeepEqual(actual["targets"], expected["targets"])
	_, envPresent := actual["env"]
	second, _ := f.run(f.repo, args...)
	after, err := os.ReadFile(f.config)
	must(err)
	idempotent := second.ExitCode == 0 && string(after) == string(data)
	return coverage && !envPresent && idempotent, reflect.DeepEqual(actual, expected) && idempotent,
		fmt.Sprintf("targets=%v; env_present=%t; second_import_unchanged=%t", actual["targets"], envPresent, idempotent)
}

func workflowFile(f *fixture, cwd, stepCwd string) string {
	path := filepath.Join(f.repo, "workflow.yaml")
	step := map[string]any{"id": "task", "agent": "codex", "prompt": "report fixture cwd"}
	if stepCwd != "" {
		step["cwd"] = stepCwd
	}
	data, err := yaml.Marshal(map[string]any{"version": 1, "name": "phase0", "cwd": cwd, "steps": []any{step}})
	must(err)
	write(path, data)
	return path
}

func resumeCwd(f *fixture) (bool, bool, string) {
	path := workflowFile(f, ".", "sub")
	o, w := f.run(f.repo, "workflow", "run", path, "--json")
	require(o.ExitCode == 0, "workflow failed")
	steps, ok := w["steps"].([]any)
	require(ok && len(steps) == 1, "missing workflow step")
	actual := steps[0].(map[string]any)["output"]
	id, ok := w["id"].(string)
	require(ok && id != "", "missing workflow id")
	r, result := f.run(f.root, "resume", id, "continue", "--step", "task", "--json")
	require(r.ExitCode == 0, "resume failed")
	want := filepath.Join(f.repo, "sub")
	return actual == want && result["output"] == f.repo, actual == want && result["output"] == want,
		fmt.Sprintf("original_cwd=%v; resumed_cwd=%v", actual, result["output"])
}

func largeEvent(f *fixture) (bool, bool, string) {
	f.configure("large", nil)
	o, result := f.run(f.repo, "run", "codex", "large fixture", "--json")
	output, _ := result["output"].(string)
	usage, hasUsage := result["usage"].(map[string]any)
	complete := o.ExitCode == 0 && result["status"] == "success" && output == strings.Repeat("x", 5*1024*1024) && hasUsage && usage["input_tokens"] == float64(7) && usage["output_tokens"] == float64(3)
	parseFailed := o.ExitCode != 0 && result["status"] == "failed" && result["exit_code"] == float64(0) && (result["parse_status"] == "truncated" || result["parse_status"] == "parse_error")
	broken := o.ExitCode == 0 && result["status"] == "success" && len(output) < 5*1024*1024 && !hasUsage && result["session_id"] == "phase0-session"
	return broken, complete || parseFailed, fmt.Sprintf("output_bytes=%d; usage_present=%t; status=%v; parse_status=%v", len(output), hasUsage, result["status"], result["parse_status"])
}

func exampleCwd(f *fixture, source string) (bool, bool, string) {
	broken, fixed := true, true
	var details []string
	for _, name := range []string{"implement-and-review.yaml", "parallel-review.yaml"} {
		data, err := os.ReadFile(filepath.Join(source, "examples", name))
		must(err)
		path := filepath.Join(f.repo, "examples", name)
		write(path, data)
		for _, caller := range []string{f.repo, f.root} {
			o, result := f.run(caller, "workflow", "run", path, "--json")
			require(o.ExitCode == 0, "example workflow failed")
			steps, ok := result["steps"].([]any)
			require(ok && len(steps) > 0, "example has no steps")
			for _, s := range steps {
				cwd := s.(map[string]any)["output"]
				broken = broken && cwd == filepath.Join(f.repo, "examples")
				fixed = fixed && cwd == f.repo
			}
			details = append(details, fmt.Sprintf("%s caller=%s cwd=%v", name, caller, result["cwd"]))
		}
	}
	return broken, fixed, strings.Join(details, "; ")
}

func doctor(f *fixture) (bool, bool, string) {
	f.configure("version-fail", nil)
	o, result := f.run(f.repo, "doctor", "codex", "--json")
	agents, ok := result["agents"].(map[string]any)
	require(ok, "doctor returned no agents")
	installation, ok := agents["codex"].(map[string]any)
	require(ok, "doctor returned no codex installation")
	diagnostic, _ := installation["error"].(string)
	valid := installation["installed"] == true && strings.Contains(diagnostic, "9")
	text, _ := f.run(f.repo, "doctor", "codex")
	return valid && o.ExitCode == 0 && text.ExitCode == 0, valid && o.ExitCode != 0 && text.ExitCode != 0,
		fmt.Sprintf("json_exit=%d; text_exit=%d; installed=%v; diagnostic=%s", o.ExitCode, text.ExitCode, installation["installed"], diagnostic)
}

func worktreeCwd(f *fixture) (bool, bool, string) {
	f.configure("marker", nil)
	path := workflowFile(f, ".", f.repo)
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Phase0 Fixture", "-c", "user.email=phase0@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}} {
		c := exec.Command("git", args...)
		c.Dir, c.Env = f.repo, f.env
		out, err := c.CombinedOutput()
		if err != nil {
			panic(fmt.Sprintf("git: %v %s", err, out))
		}
	}
	o, result := f.run(f.repo, "workflow", "run", path, "--worktree", "--json")
	require(o.ExitCode == 0, "worktree workflow failed")
	steps, ok := result["steps"].([]any)
	require(ok && len(steps) == 1, "missing worktree step")
	actual, _ := steps[0].(map[string]any)["output"].(string)
	_, sourceErr := os.Stat(filepath.Join(f.repo, "escape-marker"))
	_, destErr := os.Stat(filepath.Join(actual, "escape-marker"))
	return actual == f.repo && sourceErr == nil,
		actual != "" && actual != f.repo && strings.HasPrefix(actual, f.root+string(filepath.Separator)) && os.IsNotExist(sourceErr) && destErr == nil,
		fmt.Sprintf("step_cwd=%s; original_repository_marker=%t", actual, sourceErr == nil)
}
