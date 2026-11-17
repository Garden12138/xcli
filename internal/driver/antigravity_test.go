package driver

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
)

func TestAntigravityPermissionsAndStructuredOutput(t *testing.T) {
	// 根据 docs/v2/driver-notes.md 追加六次实测，拒绝也可能仅出现在工具事件里。
	for _, tc := range []struct{ fixture, action string }{
		{"error_plan_permission", "command"}, {"error_write_denied", "write_file"}, {"error_command_denied", "command"},
	} {
		r := replayFixture(t, "antigravity", tc.fixture, 0)
		if r.ErrorKind != ErrPermissionDenied || !reflect.DeepEqual(r.Denied, []string{tc.action}) {
			t.Fatalf("%s: %+v", tc.fixture, r)
		}
	}
	r := replayFixture(t, "antigravity", "image_ok", 0)
	var doc struct{ Files []string }
	if err := json.Unmarshal([]byte(r.Text), &doc); err != nil || len(doc.Files) != 1 || r.ErrorKind != ErrNone {
		t.Fatalf("structured output: %+v, %v", r, err)
	}
	for _, tc := range []struct{ line, action string }{
		{`{"event":"result","result":{"status":"SUCCESS","response":"ok","denied_actions":["run_command"]}}`, "run_command"},
		{`{"event":"result","result":{"status":"SUCCESS","response":"ok","denied_actions":[{"action":"command","display_name":"RunCommand"}]}}`, "command"},
	} {
		d, _ := New("antigravity", config.Default().Agents["antigravity"])
		p := d.NewParser()
		p.Feed([]byte(tc.line))
		if r := p.Finish(0, ""); r.ErrorKind != ErrPermissionDenied || r.Text != "ok" || !reflect.DeepEqual(r.Denied, []string{tc.action}) {
			t.Fatalf("denied variants: %+v", r)
		}
	}
}

func TestAntigravityEmptyTextWithReadDenial(t *testing.T) {
	// 任务17实测：命令拒绝与额外路径读取拒绝可以同时出现，最终 response 为空。
	d, _ := New("antigravity", config.Default().Agents["antigravity"])
	p := d.NewParser()
	for _, line := range []string{
		`{"event":"step_update","step_update":{"conversation_id":"s","state":"ERROR","tool_name":"run_command","tool_info":{"error":{"message":"permission check failed for command: Permission denied"}}}}`,
		`{"event":"step_update","step_update":{"conversation_id":"s","state":"ERROR","tool_name":"read_file","tool_info":{"error":{"message":"permission check failed for read_file: user denied permission"}}}}`,
		`{"event":"result","result":{"conversation_id":"s","status":"SUCCESS","response":"","denied_actions":[{"action":"read_file","display_name":"ReadFile"}]}}`,
	} {
		p.Feed([]byte(line))
	}
	r := p.Finish(0, "")
	if r.ErrorKind != ErrPermissionDenied || r.Text != "" || r.SessionID != "s" || !reflect.DeepEqual(r.Denied, []string{"command", "read_file"}) {
		t.Fatalf("read denial: %+v", r)
	}
}

func TestAntigravityReadProbeDeniedWithAnswer(t *testing.T) {
	// 任务17复测：原生读取成功、有最终文案，但附带 ls 被拒；记录当前设计的判定。
	r := replayFixture(t, "antigravity", "ask_read_probe_denied", 0)
	if r.ErrorKind != ErrPermissionDenied || r.Text != "本项目是一个用于演示与测试的基础示例项目（Demo）。\n" || !reflect.DeepEqual(r.Denied, []string{"command"}) {
		t.Fatalf("read probe denial: %+v", r)
	}
}

func TestAntigravityNativeAllowFixtures(t *testing.T) {
	// 原生候选设置只放行 ls/pwd；两个样本分别覆盖读取成功和写入拒绝。
	read := replayFixture(t, "antigravity", "native_allow_read_ok", 0)
	if read.ErrorKind != ErrNone || read.Text == "" || len(read.Denied) != 0 || read.SessionID == "" {
		t.Fatalf("native allow read: %+v", read)
	}
	write := replayFixture(t, "antigravity", "native_allow_write_checks", 0)
	if write.ErrorKind != ErrPermissionDenied || !reflect.DeepEqual(write.Denied, []string{"write_file", "command"}) {
		t.Fatalf("native allow writes: %+v", write)
	}
	redirect := replayFixture(t, "antigravity", "native_allow_ls_redirect_denied", 0)
	if redirect.ErrorKind != ErrPermissionDenied || redirect.Text != "" || !reflect.DeepEqual(redirect.Denied, []string{"command"}) {
		t.Fatalf("native allow redirect: %+v", redirect)
	}
}

func TestAllDriversRegistered(t *testing.T) {
	cfg := config.Default()
	all := All(cfg)
	if len(all) != 3 || all["cursor"].NativeSchema() || !all["antigravity"].NativeSchema() {
		t.Fatalf("registry: %+v", all)
	}
	off := false
	cfg.Agents["cursor"] = config.AgentConfig{Enabled: &off}
	if len(All(cfg)) != 2 {
		t.Fatal("disabled driver registered")
	}
	if _, err := New("other", config.AgentConfig{}); err == nil {
		t.Fatal("unknown driver accepted")
	}
}

func TestAntigravityBuild(t *testing.T) {
	d, err := New("antigravity", config.Default().Agents["antigravity"])
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		inv  Invocation
		want []string
	}{
		{"read", Invocation{Prompt: "hi", Workdir: "/w", Permission: api.PermRead},
			[]string{"--output-format", "stream-json", "--print-timeout", "0", "--sandbox", "--settings",
				`{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"allow":["command(ls)","command(pwd)"],"ask":[],"deny":["write_file(*)","mcp(*)"]}}`, "--print", "hi"}},
		{"write all options", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermWrite, Model: "m", SchemaFile: "/s.json", AddDirs: []string{"/in"}},
			[]string{"--output-format", "stream-json", "--print-timeout", "0", "--dangerously-skip-permissions", "--sandbox",
				"--model", "m", "--json-schema", "/s.json", "--add-dir", "/in", "--print", "p"}},
		{"full", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermFull},
			[]string{"--output-format", "stream-json", "--print-timeout", "0", "--dangerously-skip-permissions", "--print", "p"}},
		{"resume", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermRead, SessionID: "sid"},
			[]string{"--output-format", "stream-json", "--print-timeout", "0", "--sandbox", "--settings",
				`{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"allow":["command(ls)","command(pwd)"],"ask":[],"deny":["write_file(*)","mcp(*)"]}}`, "--conversation", "sid", "--print", "p"}},
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
func TestAntigravityRecordedFixtures(t *testing.T) {
	ask := replayFixture(t, "antigravity", "ask_ok", 0)
	if ask.ErrorKind != ErrNone || ask.Text == "" || ask.SessionID == "" || ask.Usage == nil || ask.Usage.InputTokens == 0 || ask.Usage.OutputTokens == 0 {
		t.Fatalf("ask: %+v", ask)
	}
	r := replayFixture(t, "antigravity", "schema_ok", 0)
	var doc map[string]string
	if err := json.Unmarshal([]byte(r.Text), &doc); err != nil || doc["answer"] == "" || r.ErrorKind != ErrNone {
		t.Fatalf("schema: %+v, %v", r, err)
	}
	if r := replayFixture(t, "antigravity", "resume_ok", 0); r.ErrorKind != ErrNone || r.SessionID != ask.SessionID {
		t.Fatalf("resume: %+v", r)
	}
	if r := replayFixture(t, "antigravity", "error_bad_model", 1); r.ErrorKind == ErrNone {
		t.Fatal("bad model passed")
	}
}

func TestAntigravityCapacityFailureWithStructuredImage(t *testing.T) {
	r := replayFixture(t, "antigravity", "error_capacity_with_image", 0)
	if r.ErrorKind != ErrUnavailable || r.SessionID != "capacity-smoke-session" || !json.Valid([]byte(r.Text)) || !strings.Contains(r.Error, "stdout.jsonl") {
		t.Fatalf("capacity failure with output: %+v", r)
	}
}

func TestAntigravityClassifiesResultErrorOnly(t *testing.T) {
	d, _ := New("antigravity", config.Default().Agents["antigravity"])
	for _, tc := range []struct {
		line string
		want ErrorKind
	}{
		{`{"event":"result","result":{"status":"ERROR","response":"login quota permission denied","error":"API error: UNAVAILABLE (code 503): No capacity available"}}`, ErrUnavailable},
		{`{"event":"result","result":{"status":"ERROR","response":"503 unavailable no capacity overloaded","error":"unexpected model error"}}`, ErrFailed},
		{`{"event":"result","result":{"status":"SUCCESS","response":"503 unavailable no capacity overloaded"}}`, ErrNone},
	} {
		p := d.NewParser()
		p.Feed([]byte(tc.line))
		if r := p.Finish(0, ""); r.ErrorKind != tc.want {
			t.Fatalf("response treated as failure cause: %+v, want=%s", r, tc.want)
		}
	}
}
