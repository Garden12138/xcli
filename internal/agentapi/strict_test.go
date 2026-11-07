package agentapi

import "testing"

func TestStrictRequestEnvelope(t *testing.T) {
	for _, s := range []string{`{"a":1,"a":2}`, `{"a":{"x":1,"x":2}}`, `{"a":"\ud800"}`, `{"a":"\udfff"}`, `{} {}`, `[]`, `null`, `{"a":NaN}`, `{"a":true}`} {
		var dst struct{}
		if StrictDecode([]byte(s), &dst) == nil {
			t.Errorf("accepted %s", s)
		}
	}
	var dst struct {
		A string `json:"a"`
	}
	for _, s := range []string{`{"a":"\ud83c\udf31"}`, `{"a":"正常"}`, `{"a":"\\ud800"}`} {
		if e := StrictDecode([]byte(s), &dst); e != nil {
			t.Errorf("rejected %s: %v", s, e)
		}
	}
	for _, s := range []string{`{}`, `{"a":null}`} {
		if StrictDecode([]byte(s), &dst) == nil {
			t.Errorf("missing required %s", s)
		}
	}
}
