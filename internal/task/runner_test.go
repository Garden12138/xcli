package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/runstore"
)

// Real subprocess fixture: covers actual argv/cwd, native protocol parsing,
// handoff consumption, deterministic command execution and persistence.
func TestTaskAgentHelper(t *testing.T) {
	if os.Getenv("XCLI_TASK_HELPER") != "1" {
		return
	}
	args := os.Args
	prompt := args[len(args)-1]
	root := os.Getenv("XCLI_TASK_FIXTURE")
	mode := os.Getenv("XCLI_TASK_MODE")
	adapter := os.Getenv("XCLI_TASK_ADAPTER")
	if mode == "cancel" {
		time.Sleep(30 * time.Second)
		os.Exit(9)
	}
	var output string
	switch {
	case strings.HasPrefix(prompt, "You are the xcli Task coordinator"):
		data, err := os.ReadFile(filepath.Join(root, "plan.json"))
		if err != nil {
			os.Exit(9)
		}
		output = string(data)
		if mode == "bad_plan" {
			output = "{broken"
		}
	case strings.HasPrefix(prompt, "You are the xcli Task reviewer"):
		if !strings.Contains(prompt, "worker handoff") {
			fmt.Fprintln(os.Stderr, "handoff was not consumed")
			os.Exit(9)
		}
		file := "result.txt"
		if _, err := os.Stat("calc.go"); err == nil {
			file = "calc.go"
		}
		data, err := os.ReadFile(file)
		if err != nil {
			os.Exit(9)
		}
		if file == "calc.go" {
			parts := strings.SplitN(prompt, "Task brief:\n", 2)
			var brief struct {
				Checks   []Check `json:"checks"`
				DiffFile string  `json:"diff_file"`
			}
			if len(parts) != 2 || json.Unmarshal([]byte(parts[1]), &brief) != nil {
				os.Exit(9)
			}
			if _, err := os.ReadFile(brief.DiffFile); err != nil {
				os.Exit(9)
			}
			for _, check := range brief.Checks {
				if check.Kind == "command" {
					if _, err := os.ReadFile(check.LogFile); err != nil {
						os.Exit(9)
					}
				}
			}
		}
		passed := !strings.Contains(string(data), "wrong") && !strings.Contains(string(data), "return 0")
		findings := []string{}
		if !passed {
			findings = append(findings, "wrong value in output")
		}
		output = compactJSON(map[string]any{"passed": passed, "findings": findings, "coverage": []string{"required output and input facts checked"}, "sources": []string{file}})
		if mode == "review_write" {
			_ = os.WriteFile(file, []byte("changed"), 0o600)
		}
	default:
		var brief struct {
			Output string `json:"output"`
			Kind   string `json:"kind"`
		}
		parts := strings.SplitN(prompt, "Task brief:\n", 2)
		if len(parts) != 2 {
			os.Exit(9)
		}
		_ = json.NewDecoder(strings.NewReader(parts[1])).Decode(&brief)
		repair := strings.HasPrefix(prompt, "You are the xcli Task repair")
		value := "correct"
		if mode == "repair" && !repair || mode == "exhaust" {
			value = "wrong"
		}
		file := filepath.Join(brief.Output, "result.txt")
		if brief.Kind == "project" {
			file = filepath.Join(brief.Output, "calc.go")
			n := 1
			if value == "wrong" {
				n = 0
			}
			value = fmt.Sprintf("package fixture\nfunc Value() int { return %d }\n", n)
		}
		if mode != "missing_artifact" {
			if err := os.WriteFile(file, []byte(value), 0o600); err != nil {
				os.Exit(9)
			}
		}
		unresolved := []string{}
		if mode == "pending_verification" {
			unresolved = []string{"Runtime validation has not run yet."}
		}
		output = compactJSON(Handoff{Summary: "worker handoff", Facts: []string{"input examined"}, Decisions: []string{"bounded edit"}, Sources: []string{file}, Unresolved: unresolved})
	}
	if mode == "broken_stream" {
		fmt.Println("not JSON")
		os.Exit(0)
	}
	encoder := json.NewEncoder(os.Stdout)
	if adapter == "codex" {
		_ = encoder.Encode(map[string]any{"type": "thread.started", "thread_id": "fixture-session"})
		if mode == "reconnect" {
			_ = encoder.Encode(map[string]any{"type": "error", "message": "Reconnecting... 2/5 (request timed out)"})
		}
		_ = encoder.Encode(map[string]any{"type": "item.completed", "item": map[string]any{"type": "agent_message", "text": output}})
		_ = encoder.Encode(map[string]any{"type": "turn.completed", "usage": map[string]int{"input_tokens": 10, "output_tokens": 5}})
	} else {
		_ = encoder.Encode(map[string]any{"type": "result", "subtype": "success", "result": output, "is_error": false, "session_id": "fixture-session"})
	}
	os.Exit(0)
}

func fixture(t *testing.T, mode string, project, single bool) (*Runner, Options, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "agent")
	// Both strings are generated filesystem paths; quote apostrophes for sh.
	quoted := "'" + strings.ReplaceAll(executable, "'", "'\\''") + "'"
	if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ \"$1\" = --version ]; then echo fixture-1; exit; fi\nif [ \"$1\" = --help ]; then echo 'exec --print --output-format'; exit; fi\nexec "+quoted+" -test.run=TestTaskAgentHelper -- \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Agents = map[string]config.AgentConfig{}
	for name, adapter := range map[string]string{"first": "codex", "second": "cursor"} {
		cfg.Agents[name] = config.AgentConfig{Command: script, Adapter: adapter, Env: map[string]string{"XCLI_TASK_HELPER": "1", "XCLI_TASK_FIXTURE": root, "XCLI_TASK_MODE": mode, "XCLI_TASK_ADAPTER": adapter}}
	}
	cfg.DefaultAgent = "first"
	opts := Options{Goal: "write required result including literal {{ untouched }}", Timeout: 30 * time.Second, MaxCalls: 6, Output: filepath.Join(root, "out")}
	if single {
		opts.Agent = "first"
	}
	plan := Plan{Version: 1, Reason: "fixture choices", Acceptance: []Requirement{{Path: "result.txt", Contains: []string{"correct"}}}, Actions: []Action{{Type: "delegate", Role: "worker", Agent: "first", Objective: "write result"}, {Type: "delegate", Role: "reviewer", Agent: "second", Objective: "check output against handoff"}, {Type: "complete"}}}
	if single {
		plan.Actions[1].Agent = "first"
	} else {
		plan.CollaborationRequired = true
	}
	if project {
		opts.Output = ""
		opts.Project = filepath.Join(root, "project")
		if err := os.Mkdir(opts.Project, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string]string{"go.mod": "module fixture\n\ngo 1.26\n", "calc.go": "package fixture\nfunc Value() int { return 0 }\n", "calc_test.go": "package fixture\nimport \"testing\"\nfunc TestValue(t *testing.T) { if Value()!=1 { t.Fatal(\"wrong value\") } }\n"} {
			if err := os.WriteFile(filepath.Join(opts.Project, name), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "baseline"}} {
			if _, err := git(opts.Project, args...); err != nil {
				t.Fatal(err)
			}
		}
		plan.Acceptance = []Requirement{{Path: "calc.go", Contains: []string{"return 1"}}}
		plan.Actions = append(plan.Actions[:1], append([]Action{{Type: "verify", Argv: []string{"go", "test", "./..."}, Cwd: "."}}, plan.Actions[1:]...)...)
	}
	data, _ := json.Marshal(plan)
	if err := os.WriteFile(filepath.Join(root, "plan.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	runs, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	return &Runner{Config: cfg, Registry: agent.NewRegistry(cfg), Store: store, Runs: runs}, opts, root
}

func TestTaskRecoveredNativeRetriesRemainObservable(t *testing.T) {
	r, opts, _ := fixture(t, "reconnect", false, true)
	result, err := r.Run(context.Background(), opts)
	if err != nil || result.Status != "completed" || result.Calls != 3 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	saved, err := r.Store.Load(result.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, attempt := range result.Attempts {
		record, err := r.Runs.Load(attempt.RunID)
		if err != nil || record.Status != "success" || record.RetryNotices != 1 || attempt.RetryNotices != 1 || saved.Attempts[i].RetryNotices != 1 {
			t.Fatalf("retry evidence lost: attempt=%+v record=%+v error=%v", attempt, record, err)
		}
	}
}

func TestTaskFourPathsAndRepair(t *testing.T) {
	for _, project := range []bool{false, true} {
		for _, single := range []bool{false, true} {
			t.Run(fmt.Sprintf("project=%t/single=%t", project, single), func(t *testing.T) {
				r, opts, _ := fixture(t, "repair", project, single)
				var events []Event
				r.Sink = func(e Event) error { events = append(events, e); return nil }
				result, err := r.Run(context.Background(), opts)
				if err != nil || result.Status != "completed" || result.Repairs != 1 || result.Calls != 5 {
					t.Fatalf("result=%+v error=%v", result, err)
				}
				for i, e := range events {
					if e.Sequence != uint64(i+1) || e.TaskID != result.ID {
						t.Fatalf("bad event %+v", e)
					}
				}
				if len(result.Handoffs) != 2 || len(result.Reviews) != 2 || result.Reviews[0].Passed == nil || *result.Reviews[0].Passed {
					t.Fatalf("lost failed evidence: %+v", result)
				}
				if single && result.Reviews[1].Kind != "self_check" || !single && result.Reviews[1].Kind != "cross_cli" {
					t.Fatal(result.Reviews)
				}
				saved, err := r.Store.Load(result.ID)
				if err != nil || saved.Status != "completed" {
					t.Fatalf("saved=%+v %v", saved, err)
				}
				records, err := r.Runs.List()
				if err != nil {
					t.Fatal(err)
				}
				for _, record := range records {
					if record.TaskID != result.ID || record.Cwd == "" || record.Status == "running" {
						t.Fatalf("unlinked run: %+v", record)
					}
					if single && record.Agent != "" && record.Agent != "first" {
						t.Fatal("single CLI constraint escaped")
					}
				}
				if project {
					original, _ := os.ReadFile(filepath.Join(opts.Project, "calc.go"))
					if !strings.Contains(string(original), "return 0") {
						t.Fatal("original repository changed")
					}
					if len(result.Checks) < 4 || result.DiffFile == "" {
						t.Fatal("missing command/diff evidence")
					}
				}
				for _, attempt := range result.Attempts {
					if attempt.Role == "reviewer" && len(attempt.Inputs) == 0 {
						t.Fatal("handoff consumption missing")
					}
				}
			})
		}
	}
}

func TestTaskFailureBoundaries(t *testing.T) {
	for _, mode := range []string{"bad_plan", "broken_stream", "exhaust", "review_write"} {
		t.Run(mode, func(t *testing.T) {
			r, o, _ := fixture(t, mode, false, true)
			result, err := r.Run(context.Background(), o)
			if err == nil || result.Status != "failed" || result.ExitCode == 0 || result.Calls > 6 || result.Repairs > 1 {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
	t.Run("budget", func(t *testing.T) {
		r, o, _ := fixture(t, "repair", false, true)
		o.MaxCalls = 3
		result, err := r.Run(context.Background(), o)
		if err == nil || result.Calls != 3 || result.Status != "failed" {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		r, o, _ := fixture(t, "cancel", false, true)
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		result, err := r.Run(ctx, o)
		if err == nil || result.Status == "completed" {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("events", func(t *testing.T) {
		r, o, _ := fixture(t, "", false, true)
		r.Sink = func(Event) error { return errors.New("sink unavailable") }
		result, err := r.Run(context.Background(), o)
		if err == nil || result.Status != "failed" || result.Calls != 0 {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

func TestTaskWaitingAndModeConstraints(t *testing.T) {
	t.Run("no_test", func(t *testing.T) {
		r, o, root := fixture(t, "", true, true)
		data, _ := os.ReadFile(filepath.Join(root, "plan.json"))
		var p Plan
		_ = json.Unmarshal(data, &p)
		p.Actions = append(p.Actions[:1], p.Actions[2:]...)
		data, _ = json.Marshal(p)
		_ = os.WriteFile(filepath.Join(root, "plan.json"), data, 0o600)
		result, err := r.Run(context.Background(), o)
		if err != nil || result.Status != "waiting_user" || result.Calls != 3 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("other_cli", func(t *testing.T) {
		r, o, _ := fixture(t, "", false, false)
		o.Agent = "first"
		result, err := r.Run(context.Background(), o)
		if err != nil || result.Status != "waiting_user" || result.Calls != 1 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("unknown_agent", func(t *testing.T) {
		r, o, _ := fixture(t, "", false, true)
		o.Agent = "missing"
		result, err := r.Run(context.Background(), o)
		if err == nil || result.Calls != 0 || result.Status != "failed" {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

func TestTaskCommandAndMaterialProtection(t *testing.T) {
	r, o, root := fixture(t, "", false, true)
	r.task = &Task{Cwd: root, Output: root, Candidates: []string{"first"}, Mode: "single"}
	for _, argv := range [][]string{{"sh", "-c", "echo bad"}, {"go", "run", "bad.go"}, {"go", "test", "-exec=sh"}, {"go", "test", "-C", "/tmp"}, {"go", "test", "-coverprofile=/tmp/escape"}, {"python3", "-c", "bad"}, {"npm", "install"}, {"git", "push"}, {r.Config.Agents["first"].Command, "goal"}} {
		if err := r.validateCommand(argv, root, false); err == nil {
			t.Fatalf("accepted unsafe command %v", argv)
		}
	}
	if err := r.validateCommand([]string{"go", "test", "./..."}, root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.verifyCwd("../"); err == nil {
		t.Fatal("accepted escaping cwd")
	}
	if err := os.MkdirAll(o.Output, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "plan.json"), filepath.Join(o.Output, "link")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := material(o.Output, false, ""); err == nil {
		t.Fatal("accepted symlink in material")
	}
	if _, err := requirementPath(o.Output, "../plan.json"); err == nil {
		t.Fatal("accepted artifact traversal")
	}
	if _, err := r.Store.Load("../bad"); err == nil {
		t.Fatal("accepted task traversal")
	}
}

func TestTaskSelectedSourceAndEvidenceIntegrity(t *testing.T) {
	t.Run("historical_pending_validation", func(t *testing.T) {
		r, o, _ := fixture(t, "pending_verification", true, true)
		result, err := r.Run(context.Background(), o)
		if err != nil || result.Status != "completed" {
			t.Fatalf("historical pending note overrides current passed evidence: %+v %v", result, err)
		}
	})
	t.Run("relative_reference", func(t *testing.T) {
		r, o, root := fixture(t, "", false, true)
		input := filepath.Join(root, "facts.txt")
		if err := os.WriteFile(input, []byte("facts"), 0o600); err != nil {
			t.Fatal(err)
		}
		input, _ = canonical(input)
		if err := os.Mkdir(o.Output, 0o700); err != nil {
			t.Fatal(err)
		}
		o.Output, _ = canonical(o.Output)
		if err := os.WriteFile(filepath.Join(o.Output, "report.md"), []byte("source: ../facts.txt"), 0o600); err != nil {
			t.Fatal(err)
		}
		r.task = &Task{Version: 1, ID: runstore.NewID("task"), Cwd: o.Output, Output: o.Output, Inputs: []Artifact{{Path: input, SHA256: digest([]byte("facts"))}}}
		if err := r.Store.Save(r.task); err != nil {
			t.Fatal(err)
		}
		passed, err := r.accept(context.Background(), []Requirement{{Path: "report.md", Sources: []string{input}}})
		if err != nil || !passed {
			t.Fatalf("%t %v", passed, err)
		}
		if err := os.WriteFile(input, []byte("changed"), 0o600); err != nil {
			t.Fatal(err)
		}
		passed, err = r.accept(context.Background(), []Requirement{{Path: "report.md", Sources: []string{input}}})
		if err != nil || passed {
			t.Fatalf("changed source accepted %t %v", passed, err)
		}
	})
	t.Run("stale_test", func(t *testing.T) {
		r, o, _ := fixture(t, "", true, true)
		if err := os.WriteFile(filepath.Join(o.Project, "check"), []byte("#!/bin/sh\nprintf '\\n// changed by verification\\n' >> calc.go\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "verification fixture"}} {
			if _, err := git(o.Project, args...); err != nil {
				t.Fatal(err)
			}
		}
		o.Verify = [][]string{{"./check"}}
		result, err := r.Run(context.Background(), o)
		if err == nil || result.Status != "failed" {
			t.Fatalf("%+v %v", result, err)
		}
		for _, c := range result.Checks {
			if c.Kind == "command" && c.Status != "stale" {
				t.Fatalf("not invalidated %+v", c)
			}
		}
	})
	t.Run("storage_failure", func(t *testing.T) {
		r, o, root := fixture(t, "", false, true)
		block := filepath.Join(root, "blocked")
		if err := os.WriteFile(block, []byte("blocked"), 0o600); err != nil {
			t.Fatal(err)
		}
		r.Sink = func(e Event) error {
			if e.Type == "task_started" {
				r.Store.root = block
			}
			return nil
		}
		result, err := r.Run(context.Background(), o)
		if err == nil || result.Status != "failed" || result.ExitCode == 0 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("sink_failure_during_worker", func(t *testing.T) {
		r, o, _ := fixture(t, "", false, true)
		r.Sink = func(e Event) error {
			if e.Role == "worker" && e.Type == "completed" {
				return errors.New("delivery unavailable")
			}
			return nil
		}
		result, err := r.Run(context.Background(), o)
		if err == nil || result.Status != "failed" {
			t.Fatalf("%+v %v", result, err)
		}
		records, _ := r.Runs.List()
		for _, record := range records {
			if record.Role == "worker" && record.Status == "success" {
				t.Fatal("sink failure reported success")
			}
		}
	})
}
