package agent

import (
	"strings"
	"testing"
)

const permissionDiagnostic = `jetski: no output produced — a tool required the "command" permission that headless mode cannot prompt for, so it was auto-denied. Add an allow-rule under permissions.allow in settings.json.`

func TestNativeStderrDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, adapter, stderr, output string
		failed                        bool
	}{
		{"permission denial", "antigravity", permissionDiagnostic, "", true},
		{"ordinary warning", "antigravity", "warning: command permission unavailable", "", false},
		{"other adapter", "codex", permissionDiagnostic, "", false},
		{"answer present", "antigravity", permissionDiagnostic, "answer", false},
		{"unrelated lines", "antigravity", "jetski: no output produced\nheadless mode cannot prompt\nauto-denied", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewStderrDiagnostics(tc.adapter)
			// Native writes can split the marker anywhere, including UTF-8 bytes.
			for _, b := range []byte(tc.stderr) {
				_, _ = d.Write([]byte{b})
			}
			_, _ = d.Write([]byte("\n" + strings.Repeat("diagnostic noise\n", 10000)))
			result, exit := Normalize(d.Apply(RunResult{Status: "success", ParseStatus: "complete", Output: tc.output, SessionID: "session"}), 0, false, false, nil)
			if (result.Status == "failed") != tc.failed || (exit != 0) != tc.failed || result.ExitCode != 0 || result.SessionID != "session" || result.ParseStatus != "complete" {
				t.Fatalf("%+v exit=%d", result, exit)
			}
			if tc.failed && (result.ErrorKind != "process_failed" || !strings.Contains(result.Error, "permissions.allow")) {
				t.Fatalf("missing permission diagnostic: %+v", result)
			}
			if len(d.window) > 4096 {
				t.Fatal("unbounded stderr window")
			}
		})
	}
}

func TestNativeDiagnosticPreservesHigherPriorityFailures(t *testing.T) {
	d := NewStderrDiagnostics("antigravity")
	_, _ = d.Write([]byte(permissionDiagnostic))
	for _, tc := range []struct {
		nativeExit         int
		canceled, timedOut bool
		want               string
	}{
		{9, false, false, "process_failed"},
		{0, true, false, "canceled"},
		{0, false, true, "timeout"},
	} {
		result, _ := Normalize(d.Apply(RunResult{Status: "success", ParseStatus: "complete"}), tc.nativeExit, tc.canceled, tc.timedOut, nil)
		if result.ErrorKind != tc.want || result.ExitCode != tc.nativeExit {
			t.Fatalf("%+v", result)
		}
	}
}

func TestNativeDiagnosticWithinLargeWrite(t *testing.T) {
	d := NewStderrDiagnostics("antigravity")
	data := []byte(strings.Repeat("noise\n", 20000) + permissionDiagnostic + "\n" + strings.Repeat("noise\n", 20000))
	n, err := d.Write(data)
	result := d.Apply(RunResult{Status: "success", ParseStatus: "complete"})
	if err != nil || n != len(data) || result.Status != "failed" {
		t.Fatalf("%+v n=%d err=%v", result, n, err)
	}
}
