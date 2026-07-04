//go:build darwin || linux
// +build darwin linux

package workflow

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/runstore"
)

func TestRunnerInfersDependencyFromStepOutput(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Agents["fake"] = config.AgentConfig{
		Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text",
	}
	registry := agent.NewRegistry(cfg)
	setEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	runner := Runner{Config: cfg, Registry: registry, Store: store, Progress: io.Discard, Stderr: io.Discard}
	definition := Workflow{
		Version: 1, Name: "test", Cwd: ".", MaxParallel: intPointer(2),
		Steps: []Step{
			{ID: "one", Agent: "fake", Prompt: "alpha"},
			{ID: "two", Agent: "fake", Prompt: "got {{ steps.one.output }}"},
		},
	}
	execution, err := runner.Run(
		context.Background(), filepath.Join(directory, "workflow.yaml"), definition, RunOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != "success" || len(execution.Steps) != 2 || execution.Steps[1].Output != "got alpha" {
		t.Fatalf("unexpected execution: %#v", execution)
	}
	if execution.Steps[0].OutputFile != "" {
		t.Fatal("temporary output path leaked into final execution")
	}
}

func TestRunnerDefaultsToSerialExecution(t *testing.T) {
	directory := t.TempDir()
	script := writeTestScript(t, directory, "serial-agent", "#!/bin/sh\n"+
		"root=\"$XCLI_TEST_SYNC\"\n"+
		"if [ \"$1\" = second ] && [ ! -f \"$root/first.done\" ]; then exit 8; fi\n"+
		"if [ \"$1\" = first ]; then sleep 0.1; touch \"$root/first.done\"; fi\n"+
		"printf '%s' \"$1\"\n")
	runner := newTestRunner(t, directory, script, map[string]string{"XCLI_TEST_SYNC": directory})
	definition := Workflow{Version: 1, Name: "serial-default", Steps: []Step{
		{ID: "first", Agent: "fake", Prompt: "first"},
		{ID: "second", Agent: "fake", Prompt: "second"},
	}}

	execution, err := runner.Run(
		context.Background(), filepath.Join(directory, "workflow.yaml"), definition, RunOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != "success" || execution.MaxParallel != 1 {
		t.Fatalf("unexpected execution: %#v", execution)
	}
}

func TestRunnerRunsReadyStepsInParallelAndHonorsLimit(t *testing.T) {
	directory := t.TempDir()
	script := writeTestScript(t, directory, "parallel-agent", "#!/bin/sh\n"+
		"root=\"$XCLI_TEST_SYNC\"\n"+
		"touch \"$root/$1.started\"\n"+
		"case \"$1\" in\n"+
		"  first) other=second ;;\n"+
		"  second) other=first ;;\n"+
		"  third)\n"+
		"    if [ ! -f \"$root/first.done\" ] && [ ! -f \"$root/second.done\" ]; then exit 8; fi\n"+
		"    touch \"$root/third.done\"\n"+
		"    printf '%s' \"$1\"\n"+
		"    exit 0\n"+
		"    ;;\n"+
		"  join)\n"+
		"    if [ ! -f \"$root/first.done\" ] || [ ! -f \"$root/second.done\" ] || [ ! -f \"$root/third.done\" ]; then exit 10; fi\n"+
		"    printf '%s' \"$1\"\n"+
		"    exit 0\n"+
		"    ;;\n"+
		"esac\n"+
		"count=0\n"+
		"while [ ! -f \"$root/$other.started\" ]; do\n"+
		"  count=$((count + 1))\n"+
		"  if [ \"$count\" -gt 300 ]; then exit 9; fi\n"+
		"  sleep 0.01\n"+
		"done\n"+
		"touch \"$root/$1.done\"\n"+
		"printf '%s' \"$1\"\n")
	runner := newTestRunner(t, directory, script, map[string]string{"XCLI_TEST_SYNC": directory})
	definition := Workflow{
		Version: 1, Name: "parallel-limit", MaxParallel: intPointer(2),
		Steps: []Step{
			{ID: "first", Agent: "fake", Prompt: "first"},
			{ID: "second", Agent: "fake", Prompt: "second"},
			{ID: "third", Agent: "fake", Prompt: "third"},
			{ID: "join", Agent: "fake", Prompt: "join", DependsOn: []string{"first", "second", "third"}},
		},
	}

	execution, err := runner.Run(
		context.Background(), filepath.Join(directory, "workflow.yaml"), definition, RunOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != "success" || execution.MaxParallel != 2 {
		t.Fatalf("unexpected execution: %#v", execution)
	}
	for index, id := range []string{"first", "second", "third", "join"} {
		if execution.Steps[index].ID != id || execution.Steps[index].Status != "success" {
			t.Fatalf("steps not in declaration order: %#v", execution.Steps)
		}
	}
}

func TestRunnerStopsAndSkipsAfterFailure(t *testing.T) {
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-agent")
	data := "#!/bin/sh\nif [ \"$1\" = fail ]; then exit 7; fi\nprintf '%s' \"$1\"\n"
	if err := os.WriteFile(script, []byte(data), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Agents["fake"] = config.AgentConfig{Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text"}
	setEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, _ := runstore.New()
	runner := Runner{Config: cfg, Registry: agent.NewRegistry(cfg), Store: store, Progress: io.Discard, Stderr: io.Discard}
	definition := Workflow{Version: 1, Name: "failure", Steps: []Step{
		{ID: "one", Agent: "fake", Prompt: "fail"},
		{ID: "two", Agent: "fake", DependsOn: []string{"one"}, Prompt: "never"},
	}}
	execution, err := runner.Run(
		context.Background(), filepath.Join(directory, "workflow.yaml"), definition, RunOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.ExitCode != 7 || execution.Steps[0].Status != "failed" || execution.Steps[1].Status != "skipped" {
		t.Fatalf("unexpected failure execution: %#v", execution)
	}
}

func TestRunnerFatalFailureCancelsRunningAndSkipsPending(t *testing.T) {
	directory := t.TempDir()
	script := writeTestScript(t, directory, "cancel-agent", "#!/bin/sh\n"+
		"root=\"$XCLI_TEST_SYNC\"\n"+
		"case \"$1\" in\n"+
		"  fail)\n"+
		"    count=0\n"+
		"    while [ ! -f \"$root/slow.started\" ]; do\n"+
		"      count=$((count + 1))\n"+
		"      if [ \"$count\" -gt 300 ]; then exit 9; fi\n"+
		"      sleep 0.01\n"+
		"    done\n"+
		"    exit 7\n"+
		"    ;;\n"+
		"  slow)\n"+
		"    touch \"$root/slow.started\"\n"+
		"    sleep 30\n"+
		"    ;;\n"+
		"  later)\n"+
		"    touch \"$root/later.started\"\n"+
		"    ;;\n"+
		"esac\n")
	runner := newTestRunner(t, directory, script, map[string]string{"XCLI_TEST_SYNC": directory})
	definition := Workflow{
		Version: 1, Name: "fatal-cancel", MaxParallel: intPointer(2),
		Steps: []Step{
			{ID: "fail", Agent: "fake", Prompt: "fail"},
			{ID: "slow", Agent: "fake", Prompt: "slow"},
			{ID: "later", Agent: "fake", Prompt: "later"},
		},
	}

	execution, err := runner.Run(
		context.Background(), filepath.Join(directory, "workflow.yaml"), definition, RunOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != "failed" || execution.ExitCode != 7 {
		t.Fatalf("unexpected execution status: %#v", execution)
	}
	if execution.Steps[0].Status != "failed" ||
		execution.Steps[1].Status != "canceled" ||
		execution.Steps[2].Status != "skipped" {
		t.Fatalf("unexpected step statuses: %#v", execution.Steps)
	}
	if _, err := os.Stat(filepath.Join(directory, "later.started")); !os.IsNotExist(err) {
		t.Fatalf("pending step was started: %v", err)
	}
}

func TestRunnerContinueOnErrorRunsIndependentBranch(t *testing.T) {
	directory := t.TempDir()
	script := writeTestScript(t, directory, "continue-agent", "#!/bin/sh\n"+
		"if [ \"$1\" = fail ]; then exit 7; fi\n"+
		"if [ \"$1\" = independent ]; then sleep 0.05; fi\n"+
		"printf '%s' \"$1\"\n")
	runner := newTestRunner(t, directory, script, nil)
	definition := Workflow{
		Version: 1, Name: "continue", MaxParallel: intPointer(2),
		Steps: []Step{
			{ID: "failing", Agent: "fake", Prompt: "fail", Retries: 1, ContinueOnError: true},
			{ID: "independent", Agent: "fake", Prompt: "independent"},
			{ID: "dependent", Agent: "fake", Prompt: "dependent", DependsOn: []string{"failing"}},
			{ID: "after", Agent: "fake", Prompt: "after"},
		},
	}

	execution, err := runner.Run(
		context.Background(), filepath.Join(directory, "workflow.yaml"), definition, RunOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != "failed" || execution.ExitCode != 7 {
		t.Fatalf("unexpected execution status: %#v", execution)
	}
	want := []string{"failed", "success", "skipped", "success"}
	for index, status := range want {
		if execution.Steps[index].Status != status {
			t.Fatalf("step %d status = %q, want %q: %#v", index, execution.Steps[index].Status, status, execution.Steps)
		}
	}
	if execution.Steps[0].Attempts != 2 {
		t.Fatalf("failure attempts = %d, want 2", execution.Steps[0].Attempts)
	}
}

func TestRunnerTimeoutCancelsParallelStep(t *testing.T) {
	directory := t.TempDir()
	script := writeTestScript(t, directory, "timeout-agent", "#!/bin/sh\n"+
		"root=\"$XCLI_TEST_SYNC\"\n"+
		"touch \"$root/$1.started\"\n"+
		"sleep 30\n")
	runner := newTestRunner(t, directory, script, map[string]string{"XCLI_TEST_SYNC": directory})
	definition := Workflow{
		Version: 1, Name: "timeout", MaxParallel: intPointer(2),
		Steps: []Step{
			{ID: "slow", Agent: "fake", Prompt: "slow", Timeout: "100ms"},
			{ID: "sibling", Agent: "fake", Prompt: "sibling"},
			{ID: "later", Agent: "fake", Prompt: "later"},
		},
	}

	execution, err := runner.Run(
		context.Background(), filepath.Join(directory, "workflow.yaml"), definition, RunOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != "timed_out" || execution.ExitCode != 124 {
		t.Fatalf("unexpected timeout execution: %#v", execution)
	}
	if execution.Steps[0].Status != "timed_out" ||
		execution.Steps[1].Status != "canceled" ||
		execution.Steps[2].Status != "skipped" {
		t.Fatalf("unexpected timeout steps: %#v", execution.Steps)
	}
}

func TestRunnerExternalCancellationCancelsRunningSteps(t *testing.T) {
	directory := t.TempDir()
	script := writeTestScript(t, directory, "external-cancel-agent", "#!/bin/sh\n"+
		"root=\"$XCLI_TEST_SYNC\"\n"+
		"touch \"$root/$1.started\"\n"+
		"sleep 30\n")
	runner := newTestRunner(t, directory, script, map[string]string{"XCLI_TEST_SYNC": directory})
	definition := Workflow{
		Version: 1, Name: "external-cancel", MaxParallel: intPointer(2),
		Steps: []Step{
			{ID: "first", Agent: "fake", Prompt: "first"},
			{ID: "second", Agent: "fake", Prompt: "second"},
			{ID: "later", Agent: "fake", Prompt: "later"},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			_, firstErr := os.Stat(filepath.Join(directory, "first.started"))
			_, secondErr := os.Stat(filepath.Join(directory, "second.started"))
			if firstErr == nil && secondErr == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()

	execution, err := runner.Run(
		ctx, filepath.Join(directory, "workflow.yaml"), definition, RunOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Status != "canceled" || execution.ExitCode != 130 {
		t.Fatalf("unexpected canceled execution: %#v", execution)
	}
	if execution.Steps[0].Status != "canceled" ||
		execution.Steps[1].Status != "canceled" ||
		execution.Steps[2].Status != "skipped" {
		t.Fatalf("unexpected canceled steps: %#v", execution.Steps)
	}
}

func TestRunnerRecordsParallelOutputsAndEffectiveLimit(t *testing.T) {
	directory := t.TempDir()
	script := writeTestScript(t, directory, "record-agent", "#!/bin/sh\nprintf '%s' \"$1\"\n")
	runner := newTestRunner(t, directory, script, nil)
	definition := Workflow{
		Version: 1, Name: "record", MaxParallel: intPointer(2),
		Steps: []Step{
			{ID: "first", Agent: "fake", Prompt: "first"},
			{ID: "second", Agent: "fake", Prompt: "second"},
		},
	}

	execution, err := runner.Run(
		context.Background(), filepath.Join(directory, "workflow.yaml"), definition,
		RunOptions{RecordOutput: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	record, err := runner.Store.Load(execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.MaxParallel != 2 || len(record.Steps) != 2 {
		t.Fatalf("unexpected record: %#v", record)
	}
	for index, id := range []string{"first", "second"} {
		if record.Steps[index].ID != id || record.Steps[index].OutputFile == "" {
			t.Fatalf("unexpected recorded steps: %#v", record.Steps)
		}
		if _, err := os.Stat(record.Steps[index].OutputFile); err != nil {
			t.Fatalf("recorded output missing: %v", err)
		}
	}
}

func TestValidateRejectsForwardDependency(t *testing.T) {
	cfg := config.Defaults()
	registry := agent.NewRegistry(cfg)
	definition := Workflow{Version: 1, Name: "invalid", Steps: []Step{
		{ID: "one", Agent: "codex", Prompt: "one", DependsOn: []string{"two"}},
		{ID: "two", Agent: "codex", Prompt: "two"},
	}}
	err := Validate(definition, registry, map[string]struct{}{})
	if err == nil || !strings.Contains(err.Error(), "declared earlier") {
		t.Fatalf("expected forward-dependency error, got %v", err)
	}
}

func TestValidateRejectsInvalidMaxParallel(t *testing.T) {
	cfg := config.Defaults()
	definition := Workflow{
		Version: 1, Name: "invalid", MaxParallel: intPointer(0),
		Steps: []Step{{ID: "one", Agent: "codex", Prompt: "one"}},
	}
	err := Validate(definition, agent.NewRegistry(cfg), map[string]struct{}{})
	if err == nil || !strings.Contains(err.Error(), "max_parallel") {
		t.Fatalf("expected max_parallel error, got %v", err)
	}
}

func writeTestScript(t *testing.T, directory, name, data string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(data), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestRunner(t *testing.T, directory, script string, environment map[string]string) Runner {
	t.Helper()
	cfg := config.Defaults()
	cfg.Agents["fake"] = config.AgentConfig{
		Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"},
		Output: "text", Env: environment,
	}
	setEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	return Runner{
		Config: cfg, Registry: agent.NewRegistry(cfg), Store: store,
		Progress: io.Discard, Stderr: io.Discard,
	}
}

func intPointer(value int) *int {
	return &value
}

func setEnvironment(t *testing.T, key, value string) {
	old, existed := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
