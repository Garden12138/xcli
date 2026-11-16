package driver

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
)

func TestCursorParserEdgeCases(t *testing.T) {
	parse := func(lines ...string) Result {
		d, _ := New("cursor", config.Default().Agents["cursor"])
		p := d.NewParser()
		for _, line := range lines {
			p.Feed([]byte(line))
		}
		return p.Finish(0, "")
	}
	r := parse(`{"type":"system","session_id":"c1"}`, `{"type":"assistant","message":{"content":[{"text":"working"}]}}`, `{"type":"assistant","message":{"content":[{"text":"done"}]}}`, `{"type":"result","subtype":"success","result":"workingdone","usage":{"inputTokens":7,"outputTokens":2}}`)
	if r.Text != "done" || r.SessionID != "c1" || r.ErrorKind != ErrNone || r.Usage == nil || r.Usage.InputTokens != 7 || r.Usage.TotalTokens != 9 {
		t.Fatalf("last assistant: %+v", r)
	}
	r = parse(`{"type":"result","subtype":"success","result":"fallback"}`)
	if r.Text != "fallback" || r.ErrorKind != ErrNone {
		t.Fatalf("result fallback: %+v", r)
	}
	r = parse(`{"type":"result","subtype":"error","is_error":true,"result":"Workspace Trust Required"}`)
	if r.ErrorKind != ErrTrust {
		t.Fatalf("trust: %+v", r)
	}
}

func TestCursorBuild(t *testing.T) {
	d, err := New("cursor", config.Default().Agents["cursor"])
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		inv  Invocation
		want []string
	}{
		{"read", Invocation{Prompt: "hi", Workdir: "/w", Permission: api.PermRead},
			[]string{"-p", "--output-format", "stream-json", "--trust", "--workspace", "/w", "--mode", "ask", "hi"}},
		{"write", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermWrite, Model: "m", AddDirs: []string{"/in"}},
			[]string{"-p", "--output-format", "stream-json", "--trust", "--workspace", "/w", "--force", "--sandbox", "enabled",
				"--model", "m", "--add-dir", "/in", "p"}},
		{"full", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermFull},
			[]string{"-p", "--output-format", "stream-json", "--trust", "--workspace", "/w", "--force", "--sandbox", "disabled", "p"}},
		{"resume", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermRead, SessionID: "sid"},
			[]string{"-p", "--output-format", "stream-json", "--trust", "--workspace", "/w", "--mode", "ask", "--resume", "sid", "p"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.Build(tc.inv)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv = %q, want %q, err=%v", got, tc.want, err)
			}
		})
	}
}
func TestCursorRecordedFixtures(t *testing.T) {
	ask := replayFixture(t, "cursor", "ask_ok", 0)
	if ask.ErrorKind != ErrNone || ask.Text == "" || ask.SessionID == "" || ask.Usage == nil || ask.Usage.InputTokens == 0 || ask.Usage.OutputTokens == 0 {
		t.Fatalf("ask: %+v", ask)
	}
	r := replayFixture(t, "cursor", "schema_ok", 0)
	var doc map[string]string
	if err := json.Unmarshal([]byte(r.Text), &doc); err != nil || doc["answer"] == "" || r.ErrorKind != ErrNone {
		t.Fatalf("schema: %+v, %v", r, err)
	}
	if r := replayFixture(t, "cursor", "resume_ok", 0); r.ErrorKind != ErrNone || r.SessionID != ask.SessionID {
		t.Fatalf("resume: %+v", r)
	}
	if r := replayFixture(t, "cursor", "error_bad_model", 1); r.ErrorKind == ErrNone {
		t.Fatal("bad model passed")
	}
}
