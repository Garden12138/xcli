package agent

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func TestCodexReconnectNoticesRequireSuccessfulCompletion(t *testing.T) {
	retry := `{"type":"error","message":"Reconnecting... 2/5 (request timed out)"}` + "\n"
	answer := `{"type":"item.completed","item":{"type":"agent_message","text":"answer"}}` + "\n"
	complete := `{"type":"turn.completed"}` + "\n"
	for _, tc := range []struct {
		name, stream string
		success      bool
	}{
		{"recovered", retry + retry + answer + complete, true},
		{"missing completion", retry + answer, false},
		{"failed turn", retry + answer + `{"type":"turn.failed"}`, false},
		{"unknown error before completion", retry + `{"type":"error","message":"fatal"}` + "\n" + answer + complete, false},
		{"notice after completion", answer + complete + retry, false},
		{"explicit terminal error", `{"type":"error","message":"Reconnecting... 2/5 (request timed out)","willRetry":false}` + "\n" + complete, false},
		{"explicit error flag", `{"type":"error","message":"Reconnecting... 2/5 (request timed out)","is_error":true}` + "\n" + complete, false},
		{"invalid retry range", `{"type":"error","message":"Reconnecting... 6/5 (request timed out)"}` + "\n" + complete, false},
		{"malformed stream", retry + "BAD\n" + answer + complete, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder := NewStreamDecoder("codex", "jsonl", nil)
			for _, b := range []byte(tc.stream) {
				_, _ = decoder.Write([]byte{b})
			}
			result := decoder.Finish()
			if (result.Status == "success") != tc.success {
				t.Fatalf("%+v", result)
			}
			if tc.success {
				if result.Output != "answer" || result.RetryNotices != 2 || result.ParseStatus != "complete" {
					t.Fatalf("lost recovery evidence: %+v", result)
				}
				for _, exit := range []int{1, -1} {
					if normalized, code := Normalize(result, exit, false, false, nil); code == 0 || normalized.Status == "success" {
						t.Fatalf("native failure was hidden: %+v", normalized)
					}
				}
			}
		})
	}
	if got := ParseStructured("cursor", "jsonl", []byte(retry+`{"type":"result","subtype":"success"}`)); got.Status == "success" {
		t.Fatal("Codex retry policy leaked into another adapter")
	}
}

func TestCursorCompleteAssistantMessagesSelectFinalResponse(t *testing.T) {
	answer := `{"passed":false,"findings":["first test failed"],"coverage":["source and copied logs"],"sources":["calc.go"]}`
	var stream strings.Builder
	encoder := json.NewEncoder(&stream)
	for _, text := range []string{"按评审要求，只读检查产物与证据。", answer} {
		if err := encoder.Encode(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := encoder.Encode(map[string]any{"type": "result", "subtype": "success", "session_id": "s", "result": "按评审要求，只读检查产物与证据。" + answer}); err != nil {
		t.Fatal(err)
	}
	result := ParseStructured("cursor", "jsonl", []byte(stream.String()))
	if result.Status != "success" || result.ParseStatus != "complete" || result.Output != answer || result.SessionID != "s" {
		t.Fatalf("%+v", result)
	}
	// An explicit terminal answer that is not a narration aggregate remains
	// authoritative, including streams with no assistant messages.
	stream.Reset()
	_ = encoder.Encode(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": "activity"}})
	_ = encoder.Encode(map[string]any{"type": "result", "subtype": "success", "result": answer})
	if got := ParseStructured("cursor", "jsonl", []byte(stream.String())); got.Output != answer || got.Status != "success" {
		t.Fatalf("terminal fallback lost: %+v", got)
	}
}

func TestAdapterCompletionAndFailure(t *testing.T) {
	cases := []struct{ adapter, valid, session, output string }{
		{"codex", `{"type":"thread.started","thread_id":"s"}` + "\n" + `{"type":"item.completed","item":{"type":"agent_message","text":"answer"}}` + "\n" + `{"type":"turn.completed"}`, "s", "answer"},
		{"claude", `{"type":"result","subtype":"success","is_error":false,"result":"answer","session_id":"s"}`, "s", "answer"},
		{"cursor", `{"type":"result","subtype":"success","is_error":false,"result":"answer","session_id":"s"}`, "s", "answer"},
		{"gemini", `{"type":"message","role":"assistant","content":"answer"}` + "\n" + `{"type":"result","status":"success","session_id":"s"}`, "s", "answer"},
		{"opencode", `{"type":"text","part":{"text":"answer"},"sessionID":"s"}` + "\n" + `{"type":"step_finish","part":{"reason":"stop"}}`, "s", "answer"},
		{"antigravity", `{"event":"result","result":{"status":"SUCCESS","conversation_id":"s","response":"answer","usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`, "s", "answer"},
		{"generic", `{"session_id":"s","output":"answer"}`, "s", "answer"},
	}
	for _, c := range cases {
		t.Run(c.adapter, func(t *testing.T) {
			format := "jsonl"
			if c.adapter == "generic" {
				format = "json"
			}
			decoder := NewStreamDecoder(c.adapter, format, nil)
			for _, b := range []byte(c.valid) {
				_, _ = decoder.Write([]byte{b})
			}
			result := decoder.Finish()
			if result.ParseStatus != "complete" || result.Status != "success" || result.SessionID != c.session || result.Output != c.output {
				t.Fatalf("%+v", result)
			}
			if c.adapter == "antigravity" && (result.Usage == nil || result.Usage.TotalTokens != 12) {
				t.Fatal("missing usage")
			}
			broken := ParseStructured(c.adapter, format, []byte(c.valid+"\nBAD secret-value"))
			if broken.Status != "failed" || strings.Contains(broken.Error, "secret-value") {
				t.Fatalf("unsafe parse result: %+v", broken)
			}
		})
	}
	for _, input := range []string{
		`{"type":"thread.started","thread_id":"s"}`,
		`{"type":"turn.failed","error":{"message":"secret"}}`,
		`{"type":"turn.completed"}` + "\n" + `{"type":"error"}`,
	} {
		if got := ParseStructured("codex", "jsonl", []byte(input)); got.Status != "failed" {
			t.Fatalf("incomplete/error stream succeeded: %+v", got)
		}
	}
	for adapter, input := range map[string]string{
		"cursor":      `{"type":"result","subtype":"error","is_error":true}`,
		"antigravity": `{"event":"result","result":{"status":"ERROR","error":"secret"}}`,
		"opencode":    `{"type":"step_finish","part":{"reason":"tool-calls"}}`,
	} {
		if got := ParseStructured(adapter, "jsonl", []byte(input)); got.Status != "failed" {
			t.Fatalf("%s error succeeded: %+v", adapter, got)
		}
	}
}

func TestLargeEventDrainKeepsCompletionAndUsage(t *testing.T) {
	for _, size := range []int{5 << 20, MaxEventBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			decoder := NewStreamDecoder("codex", "jsonl", nil)
			_, _ = decoder.Write([]byte(`{"type":"unknown","payload":"`))
			chunk := []byte(strings.Repeat("x", 1024))
			for n := 0; n < size; n += len(chunk) {
				_, _ = decoder.Write(chunk)
			}
			_, _ = decoder.Write([]byte("\"}\n" + `{"type":"thread.started","thread_id":"saved"}` + "\n" + `{"type":"turn.completed","usage":{"input_tokens":2,"output_tokens":3}}` + "\n"))
			result := decoder.Finish()
			expected := "complete"
			if size > MaxEventBytes {
				expected = "truncated"
			}
			if result.ParseStatus != expected || result.SessionID != "saved" || result.Usage == nil {
				t.Fatalf("%+v", result)
			}
		})
	}
	decoder := NewStreamDecoder("generic", "text", nil)
	for i := 0; i < 10; i++ {
		_, _ = decoder.Write([]byte(strings.Repeat("x", 1<<20)))
	}
	result := decoder.Finish()
	if result.ParseStatus != "truncated" || len(result.Output) != MaxOutputBytes {
		t.Fatalf("unbounded output: %d %+v", len(result.Output), result.ParseStatus)
	}
}

func TestNormalizePreservesNativeExit(t *testing.T) {
	cases := []struct {
		status            string
		native            int
		canceled, timeout bool
		want              int
	}{
		{"complete", 0, false, false, 0}, {"parse_error", 0, false, false, 1}, {"truncated", 0, false, false, 1},
		{"parse_error", 9, false, false, 9}, {"complete", 0, true, false, 130}, {"complete", -1, false, true, 124},
	}
	for _, c := range cases {
		result, code := Normalize(RunResult{ParseStatus: c.status}, c.native, c.canceled, c.timeout, nil)
		if result.ExitCode != c.native || code != c.want {
			t.Fatalf("%+v -> %+v %d", c, result, code)
		}
	}
}

func TestAntigravityDeniedActionsPreserveAnswerButFailExecution(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		wantExit    int
	}{
		{"denied with answer", `,"denied_actions":[{"action":"command","display_name":"RunCommand"}]`, 1},
		{"empty denied list", `,"denied_actions":[]`, 0},
		{"no denied field", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := `{"event":"result","result":{"status":"SUCCESS","conversation_id":"s","response":"Waiting for the image result.","usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}` + tc.field + `}}`
			parsed := ParseStructured("antigravity", "jsonl", []byte(input))
			result, code := Normalize(parsed, 0, false, false, nil)
			if code != tc.wantExit || result.ExitCode != 0 || result.ParseStatus != "complete" || result.SessionID != "s" || result.Output != "Waiting for the image result." || result.Usage == nil || result.Usage.TotalTokens != 12 {
				t.Fatalf("%+v code=%d", result, code)
			}
			if tc.wantExit == 1 && (result.Status != "failed" || result.ErrorKind != "process_failed" || !strings.Contains(result.Error, "denied tool permissions")) {
				t.Fatalf("%+v", result)
			}
		})
	}
	broken := ParseStructured("antigravity", "jsonl", []byte("BAD secret\n"+`{"event":"result","result":{"status":"SUCCESS","response":"answer","denied_actions":[{"action":"command"}]}}`))
	if broken.ParseStatus != "parse_error" || broken.ErrorKind != "parse_error" || strings.Contains(broken.Error, "secret") {
		t.Fatalf("protocol failure was hidden: %+v", broken)
	}
}

func TestCodexQuotaFailureKeepsSanitizedCauseAndNativeExit(t *testing.T) {
	for _, event := range []string{
		`{"type":"error","message":"You’ve hit your usage limit. private@example.invalid try again at 7:04 PM."}`,
		`{"type":"turn.failed","error":{"message":"private@example.invalid","codex_error_info":"usage_limit_exceeded"}}`,
	} {
		stream := `{"type":"thread.started","thread_id":"quota-session"}` + "\n" + event
		for _, native := range []int{0, 1} {
			parsed := ParseStructured("codex", "jsonl", []byte(stream))
			result, code := Normalize(parsed, native, false, false, nil)
			if code != 1 || result.ExitCode != native || result.Status != "failed" || result.ParseStatus != "complete" || result.ErrorKind != "quota_exceeded" || result.SessionID != "quota-session" || strings.Contains(result.Error, "private@example.invalid") {
				t.Fatalf("quota cause lost or private native text leaked: %+v exit=%d", result, code)
			}
			canceled, code := Normalize(parsed, native, true, false, nil)
			if code != 130 || canceled.ErrorKind != "canceled" {
				t.Fatal("quota overrode cancellation")
			}
		}
		broken := ParseStructured("codex", "jsonl", []byte("BAD secret\n"+stream))
		if broken.ErrorKind != "parse_error" || strings.Contains(broken.Error, "secret") {
			t.Fatal("quota hid protocol damage", broken)
		}
	}
	quoted := ParseStructured("codex", "jsonl", []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"You've hit your usage limit is an example diagnostic."}}`+"\n"+`{"type":"turn.completed"}`))
	if quoted.Status != "success" {
		t.Fatal("ordinary response was classified as a native quota failure")
	}
}

func BenchmarkDecoderOutput(b *testing.B) {
	for _, size := range []int{100 << 20, 1 << 30} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			line := []byte(`{"type":"unknown","payload":"` + strings.Repeat("x", 4096) + "\"}\n")
			var peak uint64
			runtime.GC()
			var baseline runtime.MemStats
			runtime.ReadMemStats(&baseline)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				decoder := NewStreamDecoder("codex", "jsonl", nil)
				for n := 0; n < size; n += len(line) {
					_, _ = decoder.Write(line)
					if n%(len(line)*4096) == 0 {
						var m runtime.MemStats
						runtime.ReadMemStats(&m)
						if m.HeapAlloc > peak {
							peak = m.HeapAlloc
						}
					}
				}
				_, _ = decoder.Write([]byte("{\"type\":\"turn.completed\"}\n"))
				if decoder.Finish().Status != "success" {
					b.Fatal("invalid benchmark stream")
				}
			}
			if peak > baseline.HeapAlloc {
				b.ReportMetric(float64(peak-baseline.HeapAlloc), "peak-extra-bytes")
			}
		})
	}
}
