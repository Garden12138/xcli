package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/stats"
)

func TestStatsCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XCLI_HOME", home)
	now := time.Now().Add(-time.Hour)
	step := stats.StepRecord{ID: "main", Kind: "ask", Agent: "codex", Status: "failed"}
	failed := stats.Record{V: 1, TaskID: "failed", App: "worklog", Status: "failed", ErrorKind: "failed", ErrorStep: "main",
		FinishedAt: now, DurationMS: 92000, Approximate: true, Steps: []stats.StepRecord{step},
		Calls: []stats.Call{{Agent: "codex", Purpose: "run", Step: "main", Round: 1, ErrorKind: "failed"}}}
	success := stats.Record{V: 1, TaskID: "done", App: "terminal", Status: "completed", FinishedAt: now, DurationMS: 92000}
	for _, r := range []stats.Record{failed, success} {
		if err := stats.Append(home, r); err != nil {
			t.Fatal(err)
		}
	}
	out, stderr, err := runCommand([]string{"stats"}, "")
	if err != nil || stderr != "" {
		t.Fatalf("stats: %v, stderr=%s", err, stderr)
	}
	for _, want := range []string{"xcli 应用统计", "按应用", "最近使用", "按 CLI", "平均耗时", "失败原因", "按天", "1m32s",
		"数据来源：账本 2 条", "其中 1 个任务", strings.Repeat("█", 20)} {
		if !strings.Contains(out, want) {
			t.Errorf("stats missing %q in %s", want, out)
		}
	}
	out, _, err = runCommand([]string{"stats", "--json", "--app", " WorkLog "}, "")
	var report stats.Report
	if err != nil || json.Unmarshal([]byte(out), &report) != nil || report.Overview.Tasks != 1 ||
		len(report.Apps) != 1 || report.Apps[0].App != "worklog" || len(report.Agents) != 1 ||
		len(report.Errors) != 1 || len(report.Daily) != 30 || report.Sources.ApproximateTasks != 1 {
		t.Fatalf("stats --json: %s, %v", out, err)
	}
	for _, field := range strings.Fields("version range since until timezone overview tasks succeeded needs_attention failed cancelled success_rate input_tokens output_tokens steps_with_usage steps apps app avg_ms p50_ms p90_ms last_used agents agent calls fallback_from fallback_to errors kind count share daily date sources ledger_records backfilled_tasks approximate_tasks skipped_lines homes") {
		if !strings.Contains(out, `"`+field+`":`) {
			t.Errorf("JSON missing %q", field)
		}
	}
}

func TestStatsDaysCollapseEmptyRuns(t *testing.T) {
	for _, tc := range []struct {
		name      string
		secondDay int
		lastRun   string
		singleDay bool
	}{
		{"consecutive task days", 19, "11-20 ～ 11-23  无任务（4 天）", false},
		{"one empty day between tasks", 20, "11-21 ～ 11-23  无任务（3 天）", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("XCLI_HOME", home)
			loc := statsLocation()
			for i, day := range []int{18, tc.secondDay} {
				at := time.Date(2026, time.November, day, 12, 0, 0, 0, loc)
				record := stats.Record{V: 1, TaskID: at.Format(time.DateOnly), App: "terminal",
					Status: "completed", FinishedAt: at, DurationMS: int64(i+1) * 1000}
				if err := stats.Append(home, record); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"stats", "--since", "30d", "--until", "2026-11-23"}
			out, _, err := runCommand(args, "")
			if err != nil || !strings.Contains(out, "10-25 ～ 11-17  无任务（24 天）") ||
				!strings.Contains(out, tc.lastRun) || strings.Contains(out, "10-26 周") {
				t.Fatalf("daily table: %v\n%s", err, out)
			}
			if tc.singleDay {
				found := false
				for line := range strings.SplitSeq(out, "\n") {
					if strings.Join(strings.Fields(line), " ") == "11-19 周四 0 0 0 —" {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing standalone empty day:\n%s", out)
				}
			}
			out, _, err = runCommand(append(args, "--json"), "")
			var report stats.Report
			if err != nil || json.Unmarshal([]byte(out), &report) != nil || len(report.Daily) != 30 {
				t.Fatalf("daily JSON: %v\n%s", err, out)
			}
			first := time.Date(2026, time.October, 25, 0, 0, 0, 0, loc)
			for i, row := range report.Daily {
				if want := first.AddDate(0, 0, i).Format(time.DateOnly); row.Date != want {
					t.Errorf("day %d = %s, want %s", i, row.Date, want)
				}
			}
			if tc.singleDay && report.Daily[25].Tasks != 0 {
				t.Fatalf("November 19 JSON = %+v", report.Daily[25])
			}
		})
	}
}

func TestStatsRanges(t *testing.T) {
	zone := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, time.November, 23, 10, 30, 0, 0, zone)
	for _, tc := range []struct {
		since, until, wantDate string
	}{
		{"7d", "", "2026-11-17"},
		{"2026-11-01", "", "2026-11-01"},
		{"all", "", ""},
		{"7d", "2026-11-19", "2026-11-13"},
	} {
		got, err := statsRange(tc.since, tc.until, now)
		date := ""
		if !got.Since.IsZero() {
			date = got.Since.Format(time.DateOnly)
		}
		if err != nil || date != tc.wantDate || got.Location != zone {
			t.Fatalf("range %q %q: %+v, %v", tc.since, tc.until, got, err)
		}
		if (tc.until == "" && !got.Until.Equal(now)) || (tc.until != "" &&
			got.Until.Format("2006-01-02 15:04:05.999999999") != "2026-11-19 23:59:59.999999999") {
			t.Fatalf("until: %s", got.Until)
		}
	}
}

func TestStatsInvalidAndEmpty(t *testing.T) {
	home := filepath.Join(t.TempDir(), "unused")
	t.Setenv("XCLI_HOME", home)
	for _, args := range [][]string{
		{"stats", "--since", "foo"}, {"stats", "--until", "2026-02-30"}, {"stats", "--app", "Bad!"},
		{"stats", "--until", ""}, {"stats", "--until", "2026-11-01", "--since", "2026-11-02"}, {"stats", "extra"}, {"stats", "--typo"},
	} {
		_, _, err := runCommand(args, "")
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != 2 || exit.Message == "" {
			t.Errorf("%v: %v", args, err)
		}
	}
	out, _, err := runCommand([]string{"stats", "--since", "all"}, "")
	if err != nil || !strings.Contains(out, "时间范围内没有任务") || strings.Contains(out, "估算") {
		t.Fatalf("empty stats: %s, %v", out, err)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stats created a data directory: %v", err)
	}
}

func TestStatsDisplayFormatting(t *testing.T) {
	for n, want := range map[int64]string{45000: "45s", 92000: "1m32s", 3900000: "1h05m"} {
		if got := statsDuration(n, true); got != want {
			t.Errorf("%d: %s, want %s", n, got, want)
		}
	}
	if statsTokens(1200000) != "1.2M" || statsTokens(34000) != "34K" || statsDisplayWidth("日期周四A") != 9 {
		t.Fatal("token or CJK width formatting differs from design")
	}
}
