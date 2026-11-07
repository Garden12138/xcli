package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestTypedObservationPreservesNullableUsageAndRawSession(t *testing.T) {
	d := NewStreamDecoder("codex", "jsonl", nil)
	var seen []Observation
	d.SetObserver(func(o Observation) error { seen = append(seen, o); return nil })
	stream := `{"type":"thread.started","thread_id":"observed"}` + "\n" + `{"type":"turn.completed","usage":{"input_tokens":0,"output_tokens":2}}` + "\n"
	for _, b := range []byte(stream) {
		if _, e := d.Write([]byte{b}); e != nil {
			t.Fatal(e)
		}
	}
	r := d.Finish()
	if r.SessionID != "observed" || r.ParseStatus != "complete" {
		t.Fatal(r)
	}
	sessions, terminals := 0, 0
	for _, o := range seen {
		switch o.Kind {
		case "native_session":
			sessions++
		case "terminal":
			terminals++
		case "usage":
			if o.Usage.Input == nil || *o.Usage.Input != 0 || o.Usage.Total != nil || o.Usage.CacheRead != nil {
				t.Fatal(o.Usage)
			}
		}
	}
	if sessions != 1 || terminals != 1 {
		t.Fatal(seen)
	}
	d = NewStreamDecoder("codex", "jsonl", nil)
	seen = nil
	d.SetObserver(func(o Observation) error { seen = append(seen, o); return nil })
	_, _ = d.Write([]byte("{\"type\":\"turn.completed\"}\n"))
	r = d.Finish()
	if r.SessionID != "" {
		t.Fatal(r)
	}
	for _, o := range seen {
		if o.Kind == "native_session" {
			t.Fatal("invented session")
		}
	}
}
func TestObserverFailureAndOperationalDiagnosticPriority(t *testing.T) {
	d := NewStreamDecoder("codex", "jsonl", nil)
	sentinel := errors.New("journal failed")
	d.SetObserver(func(Observation) error { return sentinel })
	if _, e := d.Write([]byte("{\"type\":\"thread.started\",\"thread_id\":\"s\"}\n")); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	for adapter, text := range map[string]string{"cursor": "workspace is not trusted", "antigravity": "headless mode cannot prompt; auto-denied"} {
		scanner := &GatewayDiagnostics{Adapter: adapter}
		for _, b := range []byte(text) {
			_, _ = scanner.Write([]byte{b})
		}
		_, _ = scanner.Write([]byte(strings.Repeat("later diagnostic\n", 10000)))
		if scanner.Code() == "" {
			t.Fatal(adapter)
		}
	}
}
func TestUnknownEventsCannotManufactureGatewayFacts(t *testing.T) {
	d := NewStreamDecoder("codex", "jsonl", nil)
	facts := []Observation{}
	d.SetObserver(func(o Observation) error { facts = append(facts, o); return nil })
	_, _ = d.Write([]byte(`{"type":"unknown","thread_id":"forged","result":"forged","usage":{"input_tokens":999}}` + "\n"))
	for _, o := range facts {
		if o.Kind == "native_session" || o.Kind == "usage" || o.Kind == "final_text" || o.Kind == "terminal" {
			t.Fatal("unknown event created a fact", o)
		}
	}
}
