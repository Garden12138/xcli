package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/stats"
	"github.com/Garden12138/xcli/internal/testutil"
)

func commandHome(t *testing.T) {
	t.Helper()
	commandHomeScript(t, testutil.FakeScript{"codex": {{Stdout: []string{`{"type":"thread.started","thread_id":"s"}`, `{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`, `{"type":"turn.completed"}`}, WriteFiles: map[string]string{"new.txt": "new"}}}})
}

func commandHomeScript(t *testing.T, script testutil.FakeScript) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XCLI_HOME", home)
	cfg := config.Default()
	cfg.Defaults.InheritShellProxy = false
	cfg.Path = testutil.InstallFakeCLIs(t, script)
	for name, a := range cfg.Agents {
		a.ArtifactDirs = []string{}
		cfg.Agents[name] = a
	}
	raw, err := config.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(home, "config.yaml"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func runCommand(args []string, input string) (string, string, error) {
	root := newRootCommand()
	var out, stderr bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader(input))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), stderr.String(), err
}

func TestRunAskJSON(t *testing.T) {
	commandHome(t)
	out, stderr, err := runCommand([]string{"ask", "hello", "-a", "codex", "--json"}, "")
	var r api.Result
	if err != nil || json.Unmarshal([]byte(out), &r) != nil || r.Status != api.StatusCompleted || stderr != "" {
		t.Fatalf("out=%s stderr=%s err=%v", out, stderr, err)
	}
	records := commandLedger(t)
	if len(records) != 1 || records[0].TaskID != r.TaskID || records[0].App != "terminal" || records[0].Approximate ||
		len(records[0].Calls) != 1 || records[0].Calls[0].Agent != "codex" {
		t.Fatalf("ask ledger: %+v", records)
	}
	result, _, err := runCommand([]string{"result", r.TaskID}, "")
	if err != nil || !json.Valid([]byte(result)) {
		t.Fatalf("result=%s err=%v", result, err)
	}
}

func commandLedger(t *testing.T) []stats.Record {
	t.Helper()
	records, skipped, err := stats.ReadLedger(filepath.Join(defaultHome(), "stats", "usage.jsonl"))
	if err != nil || skipped != 0 {
		t.Fatalf("ledger: skipped=%d, err=%v", skipped, err)
	}
	return records
}

func TestSubmitEvents(t *testing.T) {
	commandHome(t)
	out, _, err := runCommand([]string{"submit", "--request", "-", "--events"}, `{"capability":"ask","agent":"codex","goal":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i, line := range lines {
		var ev api.Event
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Seq != i+1 {
			t.Fatal(line)
		}
		if i == len(lines)-1 && ev.Type != "task.finished" {
			t.Fatal(line)
		}
	}
	if records := commandLedger(t); len(records) != 1 || records[0].App != "unknown" {
		t.Fatalf("submit ledger: %+v", records)
	}
}

func TestInvalidRequestJSON(t *testing.T) {
	commandHome(t)
	for _, tc := range []struct {
		args []string
		in   string
	}{
		{[]string{"ask", "--json"}, ""},
		{[]string{"submit", "--request", "-", "--json"}, `{"goal":"x","unknown":1}`},
		{[]string{"submit", "--request", "-", "--json"}, `{"goal":"x"} {"goal":"y"}`},
		{[]string{"ask", "x", "--json", "--model", "invalid"}, ""},
		{[]string{"ask", "x", "--json", "--review", "--no-review"}, ""},
		{[]string{"ask", "x", "--json", "--max-fix-rounds", "bad"}, ""},
		{[]string{"ask", "x", "--json", "--unknown-flag"}, ""},
		{[]string{"ask", "x", "--json", "--app", "Bad!"}, ""},
	} {
		out, _, err := runCommand(tc.args, tc.in)
		var r api.Result
		var exit *ExitError
		if json.Unmarshal([]byte(out), &r) != nil || r.Status != api.StatusInvalid || !errors.As(err, &exit) || exit.Code != 2 {
			t.Fatalf("args=%v out=%s err=%v", tc.args, out, err)
		}
	}
	if records := commandLedger(t); len(records) != 0 {
		t.Fatalf("invalid requests wrote usage: %+v", records)
	}
}

func TestRunCodeNonGit(t *testing.T) {
	commandHome(t)
	out, _, err := runCommand([]string{"code", "fix", "-C", t.TempDir(), "-a", "codex", "--no-review", "--json"}, "")
	var r api.Result
	if err != nil || json.Unmarshal([]byte(out), &r) != nil || r.Status != api.StatusCompleted || r.Changes == nil || len(r.Changes.Files) != 1 {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

func TestRequestFlagsPreserveUnsetBooleans(t *testing.T) {
	var f requestFlags
	r := f.Request(api.CapCode, "fix")
	if r.Review != nil || r.MaxFixRounds != nil || r.Fallback != nil {
		t.Fatalf("unset values: %+v", r)
	}
}

func TestTaskLifecycleCommands(t *testing.T) {
	commandHome(t)
	out, _, err := runCommand([]string{"ask", "hello", "-a", "codex", "--json"}, "")
	if err != nil {
		t.Fatal(err)
	}
	var r api.Result
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"status", r.TaskID, "--json"}, {"wait", r.TaskID, "--timeout", "1s"}, {"list", "--json"}} {
		out, _, err := runCommand(args, "")
		if err != nil || !json.Valid([]byte(out)) {
			t.Fatalf("%v: %s %v", args, out, err)
		}
	}
	out, _, err = runCommand([]string{"logs", r.TaskID, "-f", "--after", "2"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var ev api.Event
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Seq <= 2 {
			t.Fatal(line)
		}
	}
	out, _, err = runCommand([]string{"resume", r.TaskID, "continue", "--json"}, "")
	if err != nil || !json.Valid([]byte(out)) {
		t.Fatalf("resume: %s %v", out, err)
	}
	if _, _, err = runCommand([]string{"cancel", r.TaskID}, ""); err == nil || !strings.Contains(err.Error(), "任务已结束") {
		t.Fatal(err)
	}
}

func TestAppOverrideAndResume(t *testing.T) {
	commandHome(t)
	out, _, err := runCommand([]string{"ask", "hello", "-a", "codex", "--json", "--app", " WorkLog "}, "")
	var parent api.Result
	if err != nil || json.Unmarshal([]byte(out), &parent) != nil || parent.Metadata["app"] != "worklog" {
		t.Fatalf("parent: %s, %v", out, err)
	}
	for _, args := range [][]string{
		{"resume", parent.TaskID, "continue", "--json"},
		{"resume", parent.TaskID, "continue", "--json", "--app", "GardenFlow"},
	} {
		out, _, err := runCommand(args, "")
		if err != nil {
			t.Fatalf("resume: %s, %v", out, err)
		}
	}
	records := commandLedger(t)
	if len(records) != 3 || records[1].App != "worklog" || records[1].ParentTask != parent.TaskID ||
		records[2].App != "gardenflow" || records[2].ParentTask != parent.TaskID {
		t.Fatalf("resume ledger: %+v", records)
	}
}

func TestResultExitCodes(t *testing.T) {
	for _, tc := range []struct {
		status api.Status
		code   int
	}{{api.StatusCompleted, 0}, {api.StatusPlanned, 0}, {api.StatusFailed, 1}, {api.StatusInvalid, 2}, {api.StatusNeedsAttention, 3}, {api.StatusCancelled, 130}} {
		root := newRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		err := (outputOptions{asJSON: true}).finish(root, api.Result{Status: tc.status})
		var exit *ExitError
		code := 0
		if errors.As(err, &exit) {
			code = exit.Code
		}
		if code != tc.code || !json.Valid(out.Bytes()) {
			t.Fatalf("%s: code=%d output=%s", tc.status, code, out.String())
		}
	}
}

func TestDoAndPlanUsePlanner(t *testing.T) {
	plan := `{"reasoning":"简单需求","steps":[{"id":"main","kind":"ask","agent":"codex","instruction":"回答问题","depends_on":[],"review_by":null,"output":null}]}`
	message, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": plan}})
	for _, name := range []string{"do", "plan"} {
		t.Run(name, func(t *testing.T) {
			commandHomeScript(t, testutil.FakeScript{"codex": {{Stdout: []string{string(message), `{"type":"turn.completed"}`}}, {Stdout: []string{`{"type":"item.completed","item":{"type":"agent_message","text":"done"}}`, `{"type":"turn.completed"}`}}}})
			out, _, err := runCommand([]string{name, "回答问题", "-a", "codex", "--json"}, "")
			var r api.Result
			if err != nil || json.Unmarshal([]byte(out), &r) != nil || r.Plan == nil || r.Plan.Source != "planner" {
				t.Fatalf("%s: %s %v", name, out, err)
			}
			wantStatus, wantCalls := api.StatusCompleted, 2
			if name == "plan" {
				wantStatus, wantCalls = api.StatusPlanned, 1
			}
			if r.Status != wantStatus || len(testutil.FakeCalls(t)) != wantCalls {
				t.Fatalf("%s: %+v", name, r)
			}
		})
	}
}
