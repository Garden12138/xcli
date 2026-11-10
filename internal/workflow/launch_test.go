//go:build darwin || linux

package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Garden12138/xcli/internal/launch"
)

func TestConfiguredAliasCommandAndRetriesUseSameLedger(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "custom-cli-alias")
	if e := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0700); e != nil {
		t.Fatal(e)
	}
	r := newTestRunner(t, dir, script, nil)
	n := 0
	ref := launch.Ref{Root: filepath.Join(dir, "ledger"), Window: "fake"}
	if e := launch.Create(ref, launch.Window{ID: "fake", Limit: 2, MaxConcurrent: 1, External: launch.External{Agent: "fixture", Count: &n, Source: "fixture", Scope: "included", Confirmed: true}}); e != nil {
		t.Fatal(e)
	}
	ctx := launch.WithRef(context.Background(), ref)
	ctx = launch.WithMetadata(ctx, launch.Metadata{TaskID: "task-a", ParentTask: "parent"})
	res := r.runStep(ctx, dir, dir, Step{ID: "review", Command: script, Retries: 1}, templateContext{}, false, "run-a")
	if res.Attempts != 2 || res.LaunchID == "" {
		t.Fatal(res)
	}
	s, e := launch.Snapshot(ref)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Records) != 2 {
		t.Fatal(s)
	}
	retry := 0
	for _, v := range s.Records {
		if v.TaskID != "task-a" || v.RunID != "run-a" || v.ParentTask != "parent" || v.Phase != "review" || v.StartState != "started" {
			t.Fatal(v)
		}
		if v.Retry {
			retry++
			if v.Attempt != 2 {
				t.Fatal(v)
			}
		}
	}
	if retry != 1 {
		t.Fatal("retry not independently charged")
	}
}
