package driver

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
)

func replayFixture(t *testing.T, agent, name string, code int) Result {
	t.Helper()
	d, err := New(agent, config.Default().Agents[agent])
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", agent, name)
	raw, err := os.ReadFile(path + ".jsonl")
	if os.IsNotExist(err) {
		path = filepath.Join("testdata", agent, "synthetic-"+name)
		raw, err = os.ReadFile(path + ".jsonl")
	}
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(path + ".stderr")
	if err != nil {
		t.Fatal(err)
	}
	p := d.NewParser()
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		p.Feed(line)
	}
	return p.Finish(code, string(stderr))
}

func TestCodexRecordedFixtures(t *testing.T) {
	ask := replayFixture(t, "codex", "ask_ok", 0)
	if ask.ErrorKind != ErrNone || ask.Text == "" || ask.SessionID == "" || ask.Usage == nil || ask.Usage.InputTokens == 0 || ask.Usage.OutputTokens == 0 {
		t.Fatalf("ask: %+v", ask)
	}
	schema := replayFixture(t, "codex", "schema_ok", 0)
	var doc map[string]string
	if err := json.Unmarshal([]byte(schema.Text), &doc); err != nil || doc["answer"] == "" || schema.ErrorKind != ErrNone {
		t.Fatalf("schema: %+v, %v", schema, err)
	}
	if r := replayFixture(t, "codex", "resume_ok", 0); r.ErrorKind != ErrNone || r.SessionID != ask.SessionID {
		t.Fatalf("resume: %+v", r)
	}
	if r := replayFixture(t, "codex", "error_bad_model", 1); r.ErrorKind == ErrNone {
		t.Fatal("bad model passed")
	}
}

func TestCodexResumeOptions(t *testing.T) {
	d, _ := New("codex", config.Default().Agents["codex"])
	got, err := d.Build(Invocation{Prompt: "-prompt", Workdir: "/w", Permission: api.PermRead, SessionID: "sid", Images: []string{"/i.png"}, SchemaFile: "/s.json", AddDirs: []string{"/ignored"}, Model: "m", ExtraArgs: []string{"--enable", "x"}})
	want := []string{"exec", "resume", "--json", "--skip-git-repo-check", "-c", `sandbox_mode="read-only"`, "-m", "m", "--output-schema", "/s.json", "-i", "/i.png", "--enable", "x", "--", "sid", "-prompt"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("resume: %q, %v", got, err)
	}
	if _, err := d.Build(Invocation{Permission: "other"}); err == nil {
		t.Fatal("invalid permission passed")
	}
}

func TestCodexAuthWithoutFinalEvent(t *testing.T) {
	p := (&codex{}).NewParser()
	r := p.Finish(1, "Not logged in. Run codex login")
	if r.ErrorKind != ErrAuth || r.Error == "" {
		t.Fatalf("auth failure: %+v", r)
	}
}

func TestCodexBuild(t *testing.T) {
	d, _ := New("codex", config.Default().Agents["codex"])
	cases := []struct {
		name string
		inv  Invocation
		want []string
	}{
		{"read", Invocation{Prompt: "hi", Workdir: "/w", Permission: api.PermRead},
			[]string{"exec", "--json", "--skip-git-repo-check", "-C", "/w", "-s", "read-only", "--", "hi"}},
		{"write all options", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermWrite, Network: true, Model: "gpt-5.5",
			SchemaFile: "/s.json", Images: []string{"/a.png", "/b.png"}, AddDirs: []string{"/in"}, ExtraArgs: []string{"--enable", "x"}},
			[]string{"exec", "--json", "--skip-git-repo-check", "-C", "/w", "-s", "workspace-write",
				"-c", "sandbox_workspace_write.network_access=true", "-m", "gpt-5.5", "--output-schema", "/s.json",
				"-i", "/a.png", "-i", "/b.png", "--add-dir", "/in", "--enable", "x", "--", "p"}},
		{"full", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermFull},
			[]string{"exec", "--json", "--skip-git-repo-check", "-C", "/w", "--dangerously-bypass-approvals-and-sandbox", "--", "p"}},
		{"resume write", Invocation{Prompt: "fix", Workdir: "/w", Permission: api.PermWrite, Network: true, SessionID: "sid"},
			[]string{"exec", "resume", "--json", "--skip-git-repo-check", "-c", `sandbox_mode="workspace-write"`,
				"-c", "sandbox_workspace_write.network_access=true", "--", "sid", "fix"}},
		{"resume full", Invocation{Prompt: "fix", Workdir: "/w", Permission: api.PermFull, SessionID: "sid"},
			[]string{"exec", "resume", "--json", "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", "--", "sid", "fix"}},
	}
	for _, c := range cases {
		got, err := d.Build(c.inv)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q (err %v)", c.name, got, c.want, err)
		}
	}
}

func TestCodexParserEdgeCases(t *testing.T) {
	feed := func(lines ...string) Result {
		p := (&codex{}).NewParser()
		for _, l := range lines {
			p.Feed([]byte(l))
		}
		return p.Finish(0, "")
	}
	r := feed(`{"type":"thread.started","thread_id":"t1"}`,
		`{"type":"error","message":"Reconnecting... 1/5"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"first"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"final"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":5,"output_tokens":2}}`)
	if r.ErrorKind != ErrNone || r.Text != "final" || r.SessionID != "t1" {
		t.Fatalf("reconnect notice must not fail and last message wins: %+v", r)
	}
	r = feed(`{"type":"thread.started","thread_id":"t1"}`,
		`{"type":"turn.failed","error":{"message":"You've hit your usage limit."}}`)
	if r.ErrorKind != ErrQuota {
		t.Fatalf("want quota, got %+v", r)
	}
	r = feed(`{"type":"thread.started","thread_id":"t1"}`)
	if r.ErrorKind != ErrProtocol {
		t.Fatalf("missing turn.completed must be protocol error, got %+v", r)
	}
	r = feed(`not json`, `{"type":"turn.completed"}`)
	if r.ErrorKind != ErrNone {
		t.Fatalf("a damaged line must not fail the run: %+v", r)
	}
}
