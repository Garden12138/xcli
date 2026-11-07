package nativeadapter

import (
	"strings"
	"testing"
)

func TestAppServerStaticDenyOnlyLifecycle(t *testing.T) {
	r := RPCEvaluation{}
	if r.Request(1, "turn/start") == nil {
		t.Fatal("uninitialized turn")
	}
	if e := r.Request(1, "initialize"); e != nil {
		t.Fatal(e)
	}
	if r.Reply(2) == nil {
		t.Fatal("unmatched response")
	}
	if e := r.Reply(1); e != nil {
		t.Fatal(e)
	}
	if r.Request(2, "command/exec") == nil {
		t.Fatal("control plane exposed")
	}
	if e := r.Request(3, "turn/interrupt"); e != nil {
		t.Fatal(e)
	}
	_ = r.Reply(3)
	if r.Interrupted {
		t.Fatal("interrupt ack confused with terminal")
	}
	r.Notification("turn/completed", "interrupted")
	if !r.Interrupted {
		t.Fatal("missing terminal")
	}
	for _, method := range []string{"item/commandExecution/requestApproval", "item/fileChange/requestApproval", "command/exec", "tool/requestUserInput", "mcpServer/elicitation/request"} {
		b := string(r.ServerRequest(4, method))
		if strings.Contains(b, "accept") || strings.Contains(b, "approve") {
			t.Fatal(b)
		}
	}
}
