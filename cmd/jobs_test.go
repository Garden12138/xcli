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
