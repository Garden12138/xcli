package modelgateway

import "testing"

func TestInvalidStructuredOutputIsExplicitFailure(t *testing.T) {
	request := object(t, `{"response_format":{"type":"json_schema","json_schema":{"name":"fixture","strict":true,"schema":{"type":"object"}}}}`)
	if err := validateStructuredJSON(request, object(t, completionBody)); err == nil {
		t.Fatal("invalid structured result was accepted")
	}
}
