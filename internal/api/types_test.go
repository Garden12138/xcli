package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRequestValidate(t *testing.T) {
	for _, tc := range []struct {
		name, request, field string
	}{
		{"minimal", `{"goal":"x"}`, ""},
		{"blank goal", `{"goal":"  "}`, "goal"},
		{"version", `{"goal":"x","version":2}`, "version"},
		{"capability", `{"goal":"x","capability":"other"}`, "capability"},
		{"permission", `{"goal":"x","permission":"other"}`, "permission"},
		{"two schemas", `{"goal":"x","schema":{},"schema_file":"s.json"}`, "schema"},
		{"null schema", `{"goal":"x","schema":null,"schema_file":"s.json"}`, ""},
		{"negative fix rounds", `{"goal":"x","max_fix_rounds":-1}`, "max_fix_rounds"},
		{"zero fix rounds", `{"goal":"x","max_fix_rounds":0}`, ""},
		{"negative timeout", `{"goal":"x","timeout_sec":-1}`, "timeout_sec"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req Request
			if err := json.Unmarshal([]byte(tc.request), &req); err != nil {
				t.Fatal(err)
			}
			err := req.Validate()
			if tc.field == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error = %v, want field %s", err, tc.field)
			}
		})
	}
}

func TestHasSchemaNull(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{`{"goal":"x","schema":null}`, false},
		{`{"goal":"x"}`, false},
		{`{"goal":"x","schema":{"type":"object"}}`, true},
	} {
		var req Request
		if err := json.Unmarshal([]byte(tc.input), &req); err != nil || req.HasSchema() != tc.want {
			t.Fatalf("HasSchema(%s) = %t, err=%v", tc.input, req.HasSchema(), err)
		}
	}
}

func TestResultJSONShape(t *testing.T) {
	raw, err := json.Marshal(Result{Version: 1, Artifacts: []Artifact{}, Steps: []StepResult{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"output":null`, `"error":null`, `"artifacts":[]`, `"steps":[]`, `"version":1`} {
		if !strings.Contains(string(raw), fragment) {
			t.Errorf("result missing %s: %s", fragment, raw)
		}
	}
}

func TestNewEventRoundTrip(t *testing.T) {
	start := time.Now()
	e, err := NewEvent("t1", 3, "notice", "", "", map[string]string{"text": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var got Event
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.V != Version || got.Task != "t1" || got.Seq != 3 || got.Type != "notice" || string(got.Data) != `{"text":"hi"}` || got.TS.Before(start) || got.TS.Location() != time.UTC {
		t.Fatalf("event = %+v", got)
	}
	if _, err := NewEvent("t1", 4, "notice", "", "", make(chan int)); err == nil {
		t.Fatal("unserializable payload must return an error")
	}
}

func TestStatusFinished(t *testing.T) {
	for _, s := range []Status{StatusCompleted, StatusNeedsAttention, StatusFailed, StatusCancelled, StatusPlanned, StatusInvalid} {
		if !s.Finished() {
			t.Errorf("%s must be a finished task", s)
		}
	}
	for _, s := range []Status{StatusQueued, StatusRunning, StatusPending, StatusWaiting} {
		if s.Finished() {
			t.Errorf("%s is still active", s)
		}
	}
}
