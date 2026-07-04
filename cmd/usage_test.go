package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/runstore"
	usagereport "github.com/Garden12138/xcli/internal/usage"
)

func TestUsageCommandJSONAndText(t *testing.T) {
	setUsageEnvironment(t, "XDG_DATA_HOME", t.TempDir())
	store, err := runstore.New()
	if err != nil {
		t.Fatal(err)
	}
	cost := 0.125
	started := time.Now().UTC()
	if err := store.Save(runstore.Record{
		ID: "run-usage", Kind: "run", Agent: "claude", Cwd: t.TempDir(), StartedAt: started,
		Status: "success", Usage: &agent.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120, EstimatedCostUSD: &cost},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(runstore.Record{
		ID: "run-legacy", Kind: "run", Agent: "codex", Cwd: t.TempDir(), StartedAt: started, Status: "success",
	}); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"usage", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var report usagereport.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if report.Totals.Tasks != 2 || report.Totals.TrackedTasks != 1 || report.Totals.CostedTasks != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}

	root = newRootCommand()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"usage", "--agent", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AGENT", "EST_COST_USD", "claude", "0.125000", "1/1", "TOTAL"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("usage table %q does not contain %q", stdout.String(), want)
		}
	}
}

func TestUsageCommandRejectsNegativeDays(t *testing.T) {
	setUsageEnvironment(t, "XDG_DATA_HOME", t.TempDir())
	root := newRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"usage", "--days", "-1"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "zero or greater") {
		t.Fatalf("expected days validation error, got %v", err)
	}
}

func setUsageEnvironment(t *testing.T, key, value string) {
	t.Helper()
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
