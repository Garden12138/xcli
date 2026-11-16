//go:build unix

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProbeArgs(t *testing.T) {
	for _, tc := range []struct {
		agent, name, sid string
		want             []string
	}{
		{"codex", "ask_ok", "", []string{"exec", "--json", "--skip-git-repo-check", "-C", "/repo", "-s", "read-only", "--", "用一句话说明 add.go 中 Add 函数的作用。"}},
		{"codex", "resume_ok", "sid", []string{"exec", "resume", "--json", "--skip-git-repo-check", "-c", `sandbox_mode="read-only"`, "--", "sid", "我上一个问题问的是哪个函数？只回答函数名。"}},
		{"cursor", "write_ok", "", []string{"-p", "--output-format", "stream-json", "--trust", "--workspace", "/repo", "--force", "--sandbox", "enabled", "在当前目录创建 hello.txt，内容为 hi，不要做其他修改。"}},
		{"antigravity", "schema_ok", "", []string{"--output-format", "stream-json", "--print-timeout", "0", "--sandbox", "--settings", `{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}`, "--json-schema", "/schema.json", "--print", "你正在被 xcli 调度执行只读任务。请直接完成，用中文回复；使用 view_file、list_dir、find_by_name、grep_search 等内置只读工具读取文件，不要调用 run_command，不要修改文件，不要制定实现计划，不要请求确认。用一句话说明 add.go 的作用，按 JSON Schema 输出。"}},
	} {
		t.Run(tc.agent+"/"+tc.name, func(t *testing.T) {
			got := probeArgs(tc.agent, tc.name, "/repo", "/out", "/schema.json", tc.sid)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProbeArgsAllCases(t *testing.T) {
	for _, agent := range []string{"codex", "cursor", "antigravity"} {
		for _, name := range probeCases {
			args := probeArgs(agent, name, "/repo", "/out", "/schema.json", "sid")
			joined := strings.Join(args, " ")
			if name == "error_bad_model" && !strings.Contains(joined, "nonexistent-model-xyz") {
				t.Errorf("%s: missing bad model: %q", agent, args)
			}
			if name == "image_ok" && (!strings.Contains(joined, "image_ok-1") || !strings.Contains(joined, "/out")) {
				t.Errorf("%s: missing image output: %q", agent, args)
			}
			if name == "schema_ok" && agent == "cursor" && !strings.Contains(joined, probeSchema) {
				t.Errorf("cursor schema missing: %q", args)
			}
		}
	}
}

func TestProbeSessionID(t *testing.T) {
	for _, tc := range []struct{ agent, data, want string }{
		{"codex", "noise\n" + `{"type":"thread.started","thread_id":"t1"}`, "t1"},
		{"cursor", `{"type":"result","session_id":"c1"}`, "c1"},
		{"antigravity", `{"event":"step_update","step_update":{"conversation_id":"a1"}}`, "a1"},
		{"antigravity", `{"event":"result","result":{"conversation_id":"a2"}}`, "a2"},
		{"codex", `{"type":"turn.completed"}`, ""},
	} {
		if got := sessionID(tc.agent, []byte(tc.data)); got != tc.want {
			t.Errorf("%s session = %q, want %q", tc.agent, got, tc.want)
		}
	}
}

func TestRecordCaseUsesFakeCLI(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "fakecli")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\"\nprintf '%s\\n' \"$2\" >&2\nexit 7\n"
	if err := os.WriteFile(exe, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
	var log bytes.Buffer
	stdout, code, err := recordCase(context.Background(), exe, []string{dir + "/repo", "/test-home/private"}, dir, dir, "/test-home", dir, "ask_ok", &log)
	if err != nil || code != 7 || !strings.Contains(string(stdout), dir+"/repo") {
		t.Fatalf("record = %q, %d, %v", stdout, code, err)
	}
	for name, want := range map[string]string{"ask_ok.jsonl": "/TMP/repo\n", "ask_ok.stderr": "/HOME/private\n"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != want {
			t.Errorf("%s = %q, %v", name, data, err)
		}
	}
	if !strings.Contains(log.String(), "exit=7") || strings.Contains(log.String(), dir) {
		t.Fatalf("record log = %s", &log)
	}
}

func TestRecordCaseTimeout(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "fakecli")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nsleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, err := recordCase(ctx, exe, nil, dir, dir, "/test-home", dir, "timeout", new(bytes.Buffer))
	if err == nil || time.Since(started) > 6*time.Second {
		t.Fatalf("timeout: elapsed=%s err=%v", time.Since(started), err)
	}
}

func TestSanitize(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"path": "/home/user/tmp/repo", "home": "/home/user/file"})
	got := sanitize(raw, "/home/user/tmp", "/home/user")
	if !json.Valid(got) || strings.Contains(string(got), "/home/user") || !bytes.Contains(got, []byte("/TMP/repo")) {
		t.Fatalf("sanitized = %s", got)
	}
}

func TestSanitizeNativeHomeAndCanonicalTemp(t *testing.T) {
	dir := t.TempDir()
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", "/userhome/.codex-account@example.com")
	raw := canonical + "/image.png " + dir + "/repo /userhome/.codex-account@example.com/generated_images/a.png /userhome/file"
	got := string(sanitize([]byte(raw), dir, "/userhome"))
	if want := "/TMP/image.png /TMP/repo /CODEX_HOME/generated_images/a.png /HOME/file"; got != want {
		t.Fatalf("sanitized = %q, want %q", got, want)
	}
}

func TestSanitizeStreamingTempPathFragment(t *testing.T) {
	raw := []byte(`{"text_delta":"/private/var/folders/test/T/xcli-read-"}`)
	got := sanitize(raw, "/unused", "/userhome")
	if !json.Valid(got) || bytes.Contains(got, []byte("/private/var/folders/")) {
		t.Fatalf("temporary path fragment = %s", got)
	}
}

func TestProbeRecordsSixCasesWithFakeCLIs(t *testing.T) {
	for _, agent := range []string{"codex", "cursor", "antigravity"} {
		t.Run(agent, func(t *testing.T) {
			dir := t.TempDir()
			name := map[string]string{"codex": "codex", "cursor": "cursor-agent", "antigravity": "agy"}[agent]
			script := `#!/bin/sh
printf 'call\n' >> "$XCLI_PROBE_FAKE_LOG"
printf '%s\n' '{"type":"thread.started","thread_id":"fake","session_id":"fake","event":"result","result":{"conversation_id":"fake"}}'
`
			if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+":/bin:/usr/bin")
			t.Setenv("HOME", dir)
			t.Setenv("XCLI_PROBE_FAKE_LOG", filepath.Join(dir, "calls"))
			var log bytes.Buffer
			if err := probe(context.Background(), agent, filepath.Join(dir, "fixtures"), &log); err != nil {
				t.Fatal(err)
			}
			calls, err := os.ReadFile(filepath.Join(dir, "calls"))
			if err != nil || strings.Count(string(calls), "call\n") != 6 {
				t.Fatalf("fake calls = %q, %v", calls, err)
			}
			for _, name := range probeCases {
				for _, ext := range []string{".jsonl", ".stderr"} {
					if _, err := os.Stat(filepath.Join(dir, "fixtures", agent, name+ext)); err != nil {
						t.Errorf("missing fixture %s%s: %v", name, ext, err)
					}
				}
			}
		})
	}
}

func TestRecordedFixtures(t *testing.T) {
	for _, agent := range []string{"codex", "cursor", "antigravity"} {
		t.Run(agent, func(t *testing.T) {
			dir := filepath.Join("..", "..", "internal", "driver", "testdata", agent)
			for _, name := range probeCases {
				for _, ext := range []string{".jsonl", ".stderr"} {
					data, err := os.ReadFile(filepath.Join(dir, name+ext))
					if err != nil {
						t.Fatal(err)
					}
					for _, line := range bytes.Split(data, []byte{'\n'}) {
						if ext == ".jsonl" && len(line) > 0 && !json.Valid(line) {
							t.Errorf("%s%s has invalid JSON", name, ext)
						}
					}
					if bytes.Contains(data, []byte("/Users/")) || bytes.Contains(data, []byte("/private/var/folders/")) || bytes.Contains(data, []byte("/private/TMP")) {
						t.Errorf("%s%s: normalize native paths before committing", name, ext)
					}
				}
			}
			ask, _ := os.ReadFile(filepath.Join(dir, "ask_ok.jsonl"))
			resume, _ := os.ReadFile(filepath.Join(dir, "resume_ok.jsonl"))
			if id := sessionID(agent, ask); id == "" || id != sessionID(agent, resume) {
				t.Errorf("recorded resume session differs from ask")
			}
		})
	}
}

func TestAntigravityReadPolicyRecordings(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "driver", "testdata", "antigravity")
	for _, name := range []string{"ask_ok", "schema_ok", "resume_ok", "error_write_denied", "error_command_denied", "plan_ok"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Status           string                     `json:"status"`
				Response         string                     `json:"response"`
				StructuredOutput map[string]json.RawMessage `json:"structured_output"`
			}
			var denied bool
			for _, line := range bytes.Split(raw, []byte{'\n'}) {
				var row struct {
					Event      string          `json:"event"`
					Result     json.RawMessage `json:"result"`
					StepUpdate struct {
						State    string `json:"state"`
						ToolInfo struct {
							Error struct{ Message string } `json:"error"`
						} `json:"tool_info"`
					} `json:"step_update"`
				}
				if len(line) == 0 {
					continue
				}
				if err := json.Unmarshal(line, &row); err != nil {
					t.Fatal(err)
				}
				if row.Event == "result" {
					if err := json.Unmarshal(row.Result, &result); err != nil {
						t.Fatal(err)
					}
				}
				if row.StepUpdate.State == "ERROR" && strings.Contains(row.StepUpdate.ToolInfo.Error.Message, "Matches user-configured deny rule") {
					denied = true
				}
			}
			if result.Status != "SUCCESS" || result.Response == "" {
				t.Fatalf("missing final response: %+v", result)
			}
			if strings.HasPrefix(name, "error_") != denied {
				t.Fatalf("permission rejection = %t", denied)
			}
			if !denied && strings.Contains(result.Response, "Proceed") {
				t.Fatalf("unexpected interactive approval: %s", result.Response)
			}
			if name == "schema_ok" && result.StructuredOutput["answer"] == nil {
				t.Fatal("missing structured answer")
			}
			if name == "resume_ok" && strings.TrimSpace(result.Response) != "Add" {
				t.Fatalf("resume response = %q", result.Response)
			}
			if name == "plan_ok" {
				var steps []struct{ ID, Kind string }
				if err := json.Unmarshal(result.StructuredOutput["steps"], &steps); err != nil || len(steps) != 2 || steps[0].Kind != "ask" || steps[1].Kind != "review" {
					t.Fatalf("planner steps = %+v, %v", steps, err)
				}
			}
		})
	}
}
