//go:build darwin || linux
// +build darwin linux

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/Garden12138/xcli/internal/runstore"
	usagepkg "github.com/Garden12138/xcli/internal/usage"
)

func TestBackgroundWorkerHelper(t *testing.T) {
	if os.Getenv("XCLI_TEST_JOB_HELPER") != "1" {
		return
	}
	separator := -1
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(99)
	}
	root := newRootCommand()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetArgs(os.Args[separator+1:])
	err := root.Execute()
	if err == nil {
		os.Exit(0)
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code)
	}
	os.Exit(1)
}

func TestDetachedRunLifecycleLogsAndUsage(t *testing.T) {
	installDetachedTestHelper(t)
	capturedArgs := captureDetachedArguments(t)
	directory := t.TempDir()
	work := filepath.Join(directory, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(directory, "fake-agent")
	scriptData := "#!/bin/sh\n" +
		"printf 'progress env=%s cwd=%s args=%s\\n' \"$JOB_ENV\" \"$PWD\" \"$*\" >&2\n" +
		"sleep 0.2\n" +
		"printf 'result=%s\\n' \"$1\"\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "fake"
	cfg.Agents["fake"] = config.AgentConfig{
		Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text",
		Env: map[string]string{"JOB_ENV": "present"},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	root := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"--config", configPath, "run", "--detach", "--json", "--cwd", work,
		"hello world", "--", "--native", "two words",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("detach: %v (stderr: %s)", err, stderr.String())
	}
	if strings.Contains(strings.Join(*capturedArgs, " "), "hello world") {
		t.Fatalf("prompt leaked into worker argv: %#v", *capturedArgs)
	}
	var launched jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &launched); err != nil {
		t.Fatalf("decode launch %q: %v", stdout.String(), err)
	}
	if launched.ID == "" || launched.Agent != "fake" || launched.Status != "running" || launched.PID <= 0 || launched.LogFile == "" {
		t.Fatalf("unexpected launch view: %#v", launched)
	}

	root = newRootCommand()
	stdout.Reset()
	stderr.Reset()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"jobs", "logs", launched.ID, "--follow"})
	if err := root.Execute(); err != nil {
		t.Fatalf("follow logs: %v (stderr: %s)", err, stderr.String())
	}
	logOutput := stdout.String()
	if !strings.Contains(logOutput, "progress env=present") || !strings.Contains(logOutput, "args=hello world --native two words") || !strings.Contains(logOutput, "result=hello world") {
		t.Fatalf("unexpected job log: %q", logOutput)
	}

	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	record := waitForJob(t, store, launched.ID, "success")
	if !record.Background || record.Cwd != work || record.SelectionSource != "default" || record.Usage != nil {
		t.Fatalf("unexpected job record: %#v", record)
	}
	info, err := os.Stat(record.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("log mode = %o", info.Mode().Perm())
	}

	if err := store.Save(runstore.Record{
		ID: "run-foreground", Kind: "run", Agent: "fake", Cwd: work,
		StartedAt: time.Now().UTC(), Status: "success",
	}); err != nil {
		t.Fatal(err)
	}
	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"jobs", "list", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var listed []jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != launched.ID || listed[0].ExitCode == nil || *listed[0].ExitCode != 0 {
		t.Fatalf("unexpected jobs list: %#v", listed)
	}

	report, err := usagepkg.Build([]runstore.Record{record}, usagepkg.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.Tasks != 1 || report.Totals.TrackedTasks != 0 {
		t.Fatalf("unexpected usage report: %#v", report)
	}
}

func TestDetachedWorkflowLifecycleStateAndPrivacy(t *testing.T) {
	installDetachedTestHelper(t)
	capturedArgs := captureDetachedArguments(t)
	directory := t.TempDir()
	script := filepath.Join(directory, "workflow-agent")
	scriptData := "#!/bin/sh\n" +
		"printf 'step started\\n' >&2\n" +
		"sleep 0.25\n" +
		"printf 'step output\\n'\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["worker"] = config.AgentConfig{
		Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}, Output: "text",
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(directory, "workflow.yaml")
	workflowData := "version: 1\n" +
		"name: detached-test\n" +
		"max_parallel: 2\n" +
		"vars:\n  secret: default-value\n" +
		"steps:\n" +
		"  - id: first\n    agent: worker\n    prompt: 'first {{ vars.secret }}'\n" +
		"  - id: second\n    agent: worker\n    prompt: 'second {{ vars.secret }}'\n"
	if err := os.WriteFile(workflowPath, []byte(workflowData), 0o600); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	const secret = "workflow-super-secret"
	root := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"--config", configPath, "workflow", "run", workflowPath,
		"--detach", "--json", "--var", "secret=" + secret,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("detach workflow: %v (stderr: %s)", err, stderr.String())
	}
	var launched jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &launched); err != nil {
		t.Fatalf("decode workflow launch %q: %v", stdout.String(), err)
	}
	if launched.Kind != "workflow" || launched.Workflow != "detached-test" || launched.MaxParallel != 2 || len(launched.Steps) != 2 {
		t.Fatalf("unexpected workflow launch: %#v", launched)
	}
	if strings.Contains(strings.Join(*capturedArgs, " "), secret) || strings.Contains(strings.Join(*capturedArgs, " "), "first {{ vars.secret }}") {
		t.Fatalf("workflow input leaked into worker argv: %#v", *capturedArgs)
	}

	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	waitForStepStatus(t, store, launched.ID, "running")
	recordData, err := os.ReadFile(filepath.Join(store.Root(), launched.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(recordData), secret) || strings.Contains(string(recordData), "first {{ vars.secret }}") {
		t.Fatalf("workflow input leaked into run record: %s", recordData)
	}

	root = newRootCommand()
	stdout.Reset()
	stderr.Reset()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"jobs", "wait", launched.ID, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("wait workflow: %v (stderr: %s)", err, stderr.String())
	}
	var finished jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &finished); err != nil {
		t.Fatal(err)
	}
	if finished.Status != "success" || finished.ExitCode == nil || *finished.ExitCode != 0 || len(finished.Steps) != 2 {
		t.Fatalf("unexpected finished workflow: %#v", finished)
	}
	for _, step := range finished.Steps {
		if step.Status != "success" || step.OutputFile != "" {
			t.Fatalf("unexpected finished step: %#v", step)
		}
	}
	logData, err := os.ReadFile(finished.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "Workflow detached-test: success") || strings.Contains(string(logData), secret) {
		t.Fatalf("unexpected workflow log: %q", logData)
	}
}

func TestJobsWaitTimeoutAndFailedExitCode(t *testing.T) {
	installDetachedTestHelper(t)
	directory := t.TempDir()
	script := filepath.Join(directory, "slow-agent")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap 'exit 130' TERM INT\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "slow"
	cfg.Agents["slow"] = config.AgentConfig{Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	launched := launchTextJob(t, configPath, "wait-test")

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "wait", launched.ID, "--timeout", "30ms", "--json"})
	err := root.Execute()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 124 {
		t.Fatalf("wait timeout error = %v", err)
	}
	var waiting jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &waiting); err != nil || waiting.Status != "running" {
		t.Fatalf("wait timeout view = %#v, %v", waiting, err)
	}

	root = newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "stop", launched.ID, "--force"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	endedAt := time.Now().UTC()
	if err := store.Save(runstore.Record{
		ID: "run-failed-wait", Kind: "run", Agent: "slow", Background: true,
		Cwd: directory, StartedAt: endedAt.Add(-time.Second), EndedAt: endedAt,
		Status: "failed", ExitCode: 7,
	}); err != nil {
		t.Fatal(err)
	}
	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "wait", "run-failed-wait"})
	err = root.Execute()
	if !errors.As(err, &exitErr) || exitErr.Code != 7 || !strings.Contains(stdout.String(), "failed") {
		t.Fatalf("failed wait = %q, %v", stdout.String(), err)
	}
}

func TestDetachedWorkflowStopsRunningStepsAndSkipsPending(t *testing.T) {
	installDetachedTestHelper(t)
	directory := t.TempDir()
	script := filepath.Join(directory, "slow-workflow-agent")
	scriptData := "#!/bin/sh\n" +
		"trap 'exit 130' TERM INT\n" +
		"printf 'workflow step ready\\n' >&2\n" +
		"while :; do sleep 1; done\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["slow"] = config.AgentConfig{Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(directory, "slow.yaml")
	workflowData := "version: 1\nname: slow\nsteps:\n" +
		"  - id: running\n    agent: slow\n    prompt: first\n" +
		"  - id: pending\n    agent: slow\n    prompt: second\n"
	if err := os.WriteFile(workflowPath, []byte(workflowData), 0o600); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "workflow", "run", workflowPath, "--detach", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var launched jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &launched); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, launched.LogFile, "workflow step ready")

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "stop", launched.ID, "--timeout", "3s", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var stopped jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != "canceled" || len(stopped.Steps) != 2 || stopped.Steps[0].Status != "canceled" || stopped.Steps[1].Status != "skipped" {
		t.Fatalf("unexpected stopped workflow: %#v", stopped)
	}
}

func TestJobsDeleteAndPruneSafety(t *testing.T) {
	directory := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	manager := jobs.Manager{Store: store}
	now := time.Now().UTC()
	createFinishedJob := func(id string, endedAt time.Time) {
		t.Helper()
		files, err := manager.CreateFiles(id)
		if err != nil {
			t.Fatal(err)
		}
		files.Log.Close()
		files.Lock.Close()
		if err := store.Save(runstore.Record{
			ID: id, Kind: "run", Agent: "fake", Background: true, LogFile: files.LogPath,
			Cwd: directory, StartedAt: endedAt.Add(-time.Minute), EndedAt: endedAt,
			Status: "success",
		}); err != nil {
			t.Fatal(err)
		}
	}
	createFinishedJob("run-delete", now.Add(-48*time.Hour))
	createFinishedJob("run-old", now.Add(-24*time.Hour))
	createFinishedJob("run-new", now.Add(-time.Hour))

	root := newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetIn(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "delete", "run-delete"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "without a terminal") {
		t.Fatalf("delete without confirmation = %v", err)
	}
	root = newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "delete", "run-delete", "--yes", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("run-delete"); !os.IsNotExist(err) {
		t.Fatalf("deleted record still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), "run-delete")); !os.IsNotExist(err) {
		t.Fatalf("deleted artifacts still exist: %v", err)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "prune", "--older-than", "12h", "--dry-run", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var preview jobCleanupResult
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil || !preview.DryRun || len(preview.Candidates) != 1 || preview.Candidates[0].ID != "run-old" {
		t.Fatalf("unexpected prune preview: %#v, %v", preview, err)
	}
	if _, err := store.Load("run-old"); err != nil {
		t.Fatalf("dry run deleted old job: %v", err)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "prune", "--older-than", "12h", "--yes", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var applied jobCleanupResult
	if err := json.Unmarshal(stdout.Bytes(), &applied); err != nil || len(applied.Deleted) != 1 || applied.Deleted[0] != "run-old" {
		t.Fatalf("unexpected prune result: %#v, %v", applied, err)
	}
	if _, err := store.Load("run-old"); !os.IsNotExist(err) {
		t.Fatalf("pruned record still exists: %v", err)
	}
	if _, err := store.Load("run-new"); err != nil {
		t.Fatalf("new record was pruned: %v", err)
	}
}

func TestDetachedBuiltinNormalizesLogAndRecordsUsage(t *testing.T) {
	installDetachedTestHelper(t)
	directory := t.TempDir()
	script := filepath.Join(directory, "fake-codex")
	scriptData := "#!/bin/sh\n" +
		"printf 'native progress\\n' >&2\n" +
		"printf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"job-session\"}'\n" +
		"printf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"normalized result\"}}'\n" +
		"printf '%s\\n' '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":20,\"cached_input_tokens\":5,\"output_tokens\":4,\"reasoning_output_tokens\":1,\"total_tokens\":24}}'\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.Agents["fake-codex"] = config.AgentConfig{Adapter: "codex", Command: script}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "run", "fake-codex", "background task", "--detach", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var launched jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &launched); err != nil {
		t.Fatal(err)
	}
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	record := waitForJob(t, store, launched.ID, "success")
	if record.SessionID != "job-session" || record.Usage == nil || record.Usage.TotalTokens != 24 || record.OutputFile != "" {
		t.Fatalf("unexpected structured job record: %#v", record)
	}
	logData, err := os.ReadFile(record.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "native progress") || !strings.Contains(logText, "normalized result") || strings.Contains(logText, "thread.started") {
		t.Fatalf("structured events leaked into job log: %q", logText)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "show", launched.ID})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var shown jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.SessionID != "job-session" || shown.Usage == nil || shown.Status != "success" {
		t.Fatalf("unexpected shown job: %#v", shown)
	}

	cfg.Recording.Output = true
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "run", "fake-codex", "record raw events", "--detach", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var recordedLaunch jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &recordedLaunch); err != nil {
		t.Fatal(err)
	}
	recorded := waitForJob(t, store, recordedLaunch.ID, "success")
	if recorded.OutputFile == "" {
		t.Fatalf("raw output was not recorded: %#v", recorded)
	}
	raw, err := os.ReadFile(recorded.OutputFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "thread.started") {
		t.Fatalf("raw structured events missing: %q", string(raw))
	}
	recordedLog, err := os.ReadFile(recorded.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(recordedLog), "thread.started") {
		t.Fatalf("raw events leaked into normalized log: %q", string(recordedLog))
	}
}

func TestJobsStopGracefulForceAndIdempotent(t *testing.T) {
	installDetachedTestHelper(t)
	directory := t.TempDir()
	script := filepath.Join(directory, "slow-agent")
	scriptData := "#!/bin/sh\n" +
		"trap 'printf stopped\\n >&2; exit 130' TERM INT\n" +
		"printf ready\\n >&2\n" +
		"while :; do sleep 1; done\n"
	if err := os.WriteFile(script, []byte(scriptData), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "slow"
	cfg.Agents["slow"] = config.AgentConfig{Adapter: "generic", Command: script, RunArgs: []string{"{{ prompt }}"}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))

	graceful := launchTextJob(t, configPath, "graceful")
	waitForLog(t, graceful.LogFile, "ready")
	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "stop", graceful.ID, "--timeout", "2s", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var stopped jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != "canceled" || stopped.ExitCode == nil || *stopped.ExitCode != 130 {
		t.Fatalf("unexpected graceful stop: %#v", stopped)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "stop", graceful.ID, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var repeated jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &repeated); err != nil || repeated.Status != "canceled" {
		t.Fatalf("idempotent stop = %#v, %v", repeated, err)
	}

	forced := launchTextJob(t, configPath, "forced")
	waitForLog(t, forced.LogFile, "ready")
	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"jobs", "stop", forced.ID, "--force", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var killed jobs.View
	if err := json.Unmarshal(stdout.Bytes(), &killed); err != nil {
		t.Fatal(err)
	}
	if killed.Status != "killed" || killed.ExitCode == nil || *killed.ExitCode != 137 {
		t.Fatalf("unexpected forced stop: %#v", killed)
	}
}

func TestJobsRejectForegroundAndInvalidTimeout(t *testing.T) {
	directory := t.TempDir()
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(runstore.Record{ID: "run-front", Kind: "run", Cwd: directory}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args   []string
		needle string
	}{
		{args: []string{"jobs", "show", "run-front"}, needle: "not a background job"},
		{args: []string{"jobs", "stop", "run-front", "--timeout", "0s"}, needle: "greater than zero"},
	}
	for _, test := range tests {
		root := newRootCommand()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(test.args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), test.needle) {
			t.Fatalf("error = %v, want containing %q", err, test.needle)
		}
	}
}

func TestDetachedRunRejectsMissingCommandBeforeCreatingRecord(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.yaml")
	cfg := config.Defaults()
	cfg.DefaultAgent = "missing"
	cfg.Agents["missing"] = config.AgentConfig{
		Adapter: "generic", Command: filepath.Join(directory, "does-not-exist"),
		RunArgs: []string{"{{ prompt }}"},
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	setCommandEnvironment(t, "XDG_DATA_HOME", filepath.Join(directory, "data"))
	root := newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", configPath, "run", "--detach", "test"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "is not available") {
		t.Fatalf("unexpected error: %v", err)
	}
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 0 {
		t.Fatalf("startup validation created records: %#v, %v", records, err)
	}
}

func installDetachedTestHelper(t *testing.T) {
	t.Helper()
	previous := detachedCommand
	detachedCommand = func(_ string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=TestBackgroundWorkerHelper", "--"}, args...)
		command := exec.Command(os.Args[0], helperArgs...)
		command.Env = append(os.Environ(), "XCLI_TEST_JOB_HELPER=1")
		return command
	}
	t.Cleanup(func() { detachedCommand = previous })
}

func captureDetachedArguments(t *testing.T) *[]string {
	t.Helper()
	previous := detachedCommand
	captured := []string{}
	detachedCommand = func(name string, args ...string) *exec.Cmd {
		captured = append([]string(nil), args...)
		return previous(name, args...)
	}
	t.Cleanup(func() { detachedCommand = previous })
	return &captured
}

func waitForJob(t *testing.T, store *runstore.Store, id, status string) runstore.Record {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		record, err := store.Load(id)
		if err == nil && record.Status == status {
			return record
		}
		time.Sleep(25 * time.Millisecond)
	}
	record, err := store.Load(id)
	t.Fatalf("job %s did not reach %s: record=%#v err=%v", id, status, record, err)
	return runstore.Record{}
}

func waitForLog(t *testing.T, path, needle string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), needle) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, err := os.ReadFile(path)
	t.Fatalf("log did not contain %q: %q, %v", needle, string(data), err)
}

func waitForStepStatus(t *testing.T, store *runstore.Store, id, status string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		record, err := store.Load(id)
		if err == nil {
			for _, step := range record.Steps {
				if step.Status == status {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	record, err := store.Load(id)
	t.Fatalf("job %s had no %s step: record=%#v err=%v", id, status, record, err)
}

func launchTextJob(t *testing.T, configPath, prompt string) jobs.View {
	t.Helper()
	root := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--config", configPath, "run", "--detach", prompt})
	if err := root.Execute(); err != nil {
		t.Fatalf("launch: %v (stderr: %s)", err, stderr.String())
	}
	fields := strings.Fields(stdout.String())
	if len(fields) != 5 || fields[0] != "Started" || fields[1] != "job" || fields[3] != "(pid" {
		t.Fatalf("unexpected text launch: %q", stdout.String())
	}
	store, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load(fields[2])
	if err != nil {
		t.Fatal(err)
	}
	return jobs.ToView(record)
}
