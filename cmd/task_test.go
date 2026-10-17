package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/task"
)

func taskCLIConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(dir, "data"))
	plan := task.Plan{Version: 1, Reason: "single CLI fixture", Acceptance: []task.Requirement{{Path: "result.txt", Contains: []string{"correct"}}}, Actions: []task.Action{{Type: "delegate", Role: "worker", Agent: "test", Objective: "produce result.txt"}, {Type: "delegate", Role: "reviewer", Agent: "test", Objective: "review result.txt"}, {Type: "complete"}}}
	response := func(value any) string {
		text, _ := json.Marshal(value)
		item, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": string(text)}})
		return "printf '%s\\n' '" + strings.ReplaceAll(string(item), "'", "'\\''") + "' '{\"type\":\"turn.completed\"}'\n"
	}
	passed := true
	body := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo fixture-1; exit; fi\nif [ \"$1\" = --help ]; then echo exec; exit; fi\ncase \"$*\" in\n*'Task coordinator'*)\n" + response(plan) + ";;\n*'Task reviewer'*)\n" + response(task.Review{Passed: &passed, Findings: []string{}, Coverage: []string{"result checked"}, Sources: []string{"result.txt"}}) + ";;\n*)\nprintf correct > result.txt\n" + response(task.Handoff{Summary: "file produced", Facts: []string{"output exists"}, Sources: []string{"result.txt"}}) + ";;\nesac\n"
	script := filepath.Join(dir, "agent")
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Agents["test"] = config.AgentConfig{Adapter: "codex", Command: script}
	path := filepath.Join(dir, "config.yaml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTaskCLIJSONEventsAndStatus(t *testing.T) {
	for _, mode := range []string{"--json", "--events"} {
		t.Run(mode, func(t *testing.T) {
			path := taskCLIConfig(t)
			root := newRootCommand()
			var out, diagnostics bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&diagnostics)
			root.SetArgs([]string{"--config", path, "task", "write a file", "--agent", "test", mode})
			if err := root.Execute(); err != nil {
				t.Fatalf("%v %s", err, diagnostics.String())
			}
			var result task.Task
			decoder := json.NewDecoder(&out)
			if mode == "--json" {
				if err := decoder.Decode(&result); err != nil {
					t.Fatal(err)
				}
				if decoder.More() {
					t.Fatal("extra JSON output")
				}
			} else {
				var sequence uint64
				for decoder.More() {
					var event task.Event
					if err := decoder.Decode(&event); err != nil {
						t.Fatal(err)
					}
					sequence++
					if event.Sequence != sequence {
						t.Fatal(event)
					}
					if event.Type == "task_finished" {
						data, _ := json.Marshal(event.Result)
						_ = json.Unmarshal(data, &result)
						if result.Sequence != event.Sequence {
							t.Fatal("final sequence mismatch")
						}
					}
				}
			}
			if result.Status != "completed" || result.Calls != 3 {
				t.Fatalf("%+v", result)
			}
			root = newRootCommand()
			out.Reset()
			root.SetOut(&out)
			root.SetArgs([]string{"task", "status", result.ID, "--json"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			var saved task.Task
			if err := json.Unmarshal(out.Bytes(), &saved); err != nil || saved.ID != result.ID || saved.Status != "completed" {
				t.Fatalf("%s %v", out.String(), err)
			}
		})
	}
}

func TestTaskCLIInvalidOptionsAndRetirement(t *testing.T) {
	for _, args := range [][]string{{"task", "goal", "--json", "--events"}, {"task", "goal", "--verify", "echo bad"}, {"task", "goal", "--max-calls", "0"}, {"task", "goal", "--timeout", "0s"}, {"task", "goal", "--detach"}, {"task", "continue", "task-test"}, {"install"}, {"auth"}, {"default"}, {"acp"}, {"route"}, {"workflow"}, {"usage"}, {"mcp", "import"}, {"mcp", "sync"}, {"mcp", "plan"}} {
		root := newRootCommand()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, name := range []string{"task", "run", "use", "resume", "jobs", "runs", "agents", "doctor", "config"} {
		command, _, err := newRootCommand().Find([]string{name})
		if err != nil || command.Name() != name {
			t.Fatalf("lost %s", name)
		}
	}
	root := newRootCommand()
	root.SetArgs([]string{"task", "status", "../bad"})
	if err := root.Execute(); err == nil {
		t.Fatal("accepted traversal")
	}
}
