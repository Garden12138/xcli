package codexrpc

import (
	"encoding/json"
	"testing"
)

func TestDecodeBoundedStrictEnvelope(t *testing.T) {
	for _, raw := range []string{`{}`, `[]`, `{"id":null,"result":{}}`, `{"id":true,"result":{}}`, `{"id":1.5,"result":{}}`, `{"id":1,"id":2,"result":{}}`, `{"id":1,"result":{},"error":{}}`, `{"method":"x","params":null}`, `{"method":"x","params":{},"emittedAtMs":"x"}`, `{"method":"x","params":{},"jsonrpc":"1.0"}`, `{"id":1,"result":{},"command":"exec"}`, `{"method":"x","params":{"bad":"\ud800"}}`} {
		if _, e := Decode([]byte(raw)); e == nil {
			t.Fatal("accepted malformed envelope", raw)
		}
	}
	for _, raw := range []string{`{"id":1,"result":{}}`, `{"method":"notification","params":{},"emittedAtMs":1234}`, `{"jsonrpc":"2.0","id":"srv","method":"approval","params":{}}`} {
		if _, e := Decode([]byte(raw)); e != nil {
			t.Fatal(e, raw)
		}
	}
}
func TestProbeCannotSendTurnThreadOrAuth(t *testing.T) {
	c := Client{probe: true, pending: map[string]string{}}
	for _, method := range []string{"turn/start", "thread/start", "thread/resume", "account/read", "account/login/start", "config/value/write", "process/spawn"} {
		if _, e := c.Request(method, map[string]any{}); e == nil {
			t.Fatal(method)
		}
	}
	task := Client{pending: map[string]string{}}
	for _, method := range []string{"command/exec", "fs/readFile", "account/read", "config/value/write", "process/spawn"} {
		if _, e := task.Request(method, map[string]any{}); e == nil {
			t.Fatal(method)
		}
	}
	c.pending["1"] = "initialize"
	if _, e := c.Accept(Message{ID: json.RawMessage(`2`)}); e == nil {
		t.Fatal("unmatched reply")
	}
	if _, e := c.Accept(Message{ID: json.RawMessage(`1`)}); e != nil {
		t.Fatal(e)
	}
	if _, e := c.Accept(Message{ID: json.RawMessage(`1`)}); e == nil {
		t.Fatal("duplicate reply")
	}
}
