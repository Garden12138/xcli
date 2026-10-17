//go:build darwin || linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/runstore"
)

// These tests check the committed compatibility examples; they do not simulate
// a future implementation and must not be reported as F02/F04/F05 fixes.
func TestContractExamplesPreserveLegacyReaders(t *testing.T) {
	data, err := os.ReadFile("testdata/contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	var samples map[string]json.RawMessage
	if err := json.Unmarshal(data, &samples); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old_run", "old_workflow", "new_workflow"} {
		var record runstore.Record
		if err := json.Unmarshal(samples[name], &record); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if record.ID == "" || record.Cwd != "/fixture/repo" || record.StartedAt.IsZero() || record.Status != "success" {
			t.Fatalf("%s lost legacy fields: %#v", name, record)
		}
	}
	var old, next agent.RunResult
	if err := json.Unmarshal(samples["old_result"], &old); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(samples["new_result"], &next); err != nil {
		t.Fatal(err)
	}
	if next.RunID == "" || next.ParseStatus != "complete" {
		t.Fatal("new result fields were lost")
	}
	projection := next
	projection.RunID = ""
	projection.ParseStatus = ""
	projection.ErrorKind = ""
	projection.Error = ""
	if !reflect.DeepEqual(old, projection) {
		t.Fatal("additive result fields changed the legacy projection")
	}
	var broken struct {
		CLIExit int             `json:"cli_exit_code"`
		Result  agent.RunResult `json:"result"`
		Record  runstore.Record `json:"record"`
	}
	if err := json.Unmarshal(samples["parse_failure"], &broken); err != nil {
		t.Fatal(err)
	}
	if broken.CLIExit != 1 || broken.Result.ExitCode != 0 || broken.Record.ExitCode != 0 || broken.Record.Status != "failed" || broken.Result.Status != "failed" {
		t.Fatal("sample conflates native exit with xcli parse failure")
	}
	var job map[string]any
	if err := json.Unmarshal(samples["initial_job"], &job); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"exit_code", "ended_at", "parse_status", "output"} {
		if _, present := job[field]; present {
			t.Fatalf("initial job invents final field %s", field)
		}
	}
	var legacy map[string]any
	if err := json.Unmarshal(samples["old_workflow"], &legacy); err != nil {
		t.Fatal(err)
	}
	if _, present := legacy["steps"].([]any)[0].(map[string]any)["cwd"]; present {
		t.Fatal("legacy sample must lack step cwd")
	}
	for _, name := range []string{"ownership_v1", "ownership_v2", "ownership_v3_unconfirmed"} {
		var value any
		if err := json.Unmarshal(samples[name], &value); err != nil {
			t.Fatal(err)
		}
		var walk func(any)
		walk = func(v any) {
			if m, ok := v.(map[string]any); ok {
				for key, child := range m {
					if key == "native_fingerprint" {
						t.Fatalf("%s silently establishes full ownership", name)
					}
					walk(child)
				}
			}
		}
		walk(value)
	}
}

func TestSmokeSetupFailureDoesNotSucceedOrInvokeModel(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	output := filepath.Join(root, "smoke.json")
	// A nonexistent native executable fails before auth or any model invocation.
	if err := realSmoke(self, "synthetic", filepath.Join(root, "missing-codex"), output); err == nil {
		t.Fatal("blocked smoke returned success")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var report smokeReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "blocked" || report.Invocations != 0 || report.Detail == "" {
		t.Fatalf("invalid blocked evidence: %#v", report)
	}
	if err := realSmoke(self, "synthetic", filepath.Join(root, "missing-codex"), output); err == nil {
		t.Fatal("existing report could be overwritten")
	}
	after, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(data) {
		t.Fatal("second attempt changed existing evidence")
	}
}
