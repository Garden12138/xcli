package stats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestAppendConcurrentAndPrivate(t *testing.T) {
	home := t.TempDir()
	task, events := fixtureTask()
	r, ok := FromTask(task, events, nil)
	if !ok {
		t.Fatal("fixture task did not convert")
	}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			entry := r
			entry.TaskID = fmt.Sprint(i)
			if err := Append(home, entry); err != nil {
				t.Errorf("Append: %v", err)
			}
		})
	}
	wg.Wait()
	path := filepath.Join(home, "stats", "usage.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(raw, []byte{'\n'}); got != 20 || bytes.Contains(raw, []byte("private")) {
		t.Fatalf("ledger lines = %d, or private fixture data leaked", got)
	}
	for line := range bytes.SplitSeq(bytes.TrimSpace(raw), []byte{'\n'}) {
		root := checkKeys(t, line, "v task_id app capability status error_kind error_step started_at finished_at duration_ms parent_task planner steps reviews calls approximate")
		for key, allowed := range map[string]string{
			"steps":   "id kind agent status duration_ms rounds fallbacks input_tokens output_tokens has_usage",
			"reviews": "step agent round verdict",
			"calls":   "agent purpose step round duration_ms error_kind input_tokens output_tokens has_usage",
		} {
			var entries []json.RawMessage
			if err := json.Unmarshal(root[key], &entries); err != nil || len(entries) == 0 {
				t.Fatalf("%s: %s, %v", key, root[key], err)
			}
			for _, entry := range entries {
				fields := checkKeys(t, entry, allowed)
				if key == "steps" {
					var fallbacks []json.RawMessage
					if err := json.Unmarshal(fields["fallbacks"], &fallbacks); err != nil {
						t.Fatal(err)
					}
					for _, fallback := range fallbacks {
						checkKeys(t, fallback, "agent reason")
					}
				}
			}
		}
	}
	for p, want := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s: %v, %v", p, info, err)
		}
	}
}

func checkKeys(t *testing.T, raw []byte, allowed string) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	keys := strings.Fields(allowed)
	if len(fields) != len(keys) {
		t.Fatalf("fields = %s, want %s", raw, allowed)
	}
	for key := range fields {
		if !slices.Contains(keys, key) {
			t.Fatalf("unexpected field %q", key)
		}
	}
	return fields
}

func TestReadLedgerSkipsBadLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	raw := "{bad}\n{\"v\":1,\"task_id\":\"a\"}\nnull\n" + strings.Repeat("x", 128*1024) + "\n{\"v\":1,\"task_id\":\"b\"}"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	got, skipped, err := ReadLedger(path)
	if err != nil || skipped != 3 || len(got) != 2 || got[0].TaskID != "a" || got[1].TaskID != "b" {
		t.Fatalf("records = %+v, skipped = %d, err = %v", got, skipped, err)
	}
}
