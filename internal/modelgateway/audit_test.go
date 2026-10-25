package modelgateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/model"
	"github.com/Garden12138/xcli/internal/modelbackend/fake"
)

func TestAuditDoesNotContainContentTokensOrUpstreamErrors(t *testing.T) {
	b := &fake.Backend{CompleteFunc: func(context.Context, model.Object) (model.Object, error) {
		return nil, model.Failure(429, "upstream_rate_limit", "quota exceeded")
	}}
	s := chatServer(t, b, 2, time.Second)
	var records []AuditRecord
	s.options.Audit = func(record AuditRecord) { records = append(records, record) }
	w := post(t, s, strings.Replace(chatBody, "synthetic", "PRIVATE_CHAT_SENTINEL", 1))
	if w.Code != 429 {
		t.Fatal(w.Code, w.Body.String())
	}
	data, _ := json.Marshal(records)
	for _, secret := range []string{"PRIVATE_CHAT_SENTINEL", "fixture-token", "temperature", "中文", "upstream_model"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("audit leaked content", string(data))
		}
	}
	if len(records) != 1 || records[0].ErrorCode != "upstream_rate_limit" || records[0].Status != 429 || records[0].RequestID == "" {
		t.Fatal(records)
	}
}
