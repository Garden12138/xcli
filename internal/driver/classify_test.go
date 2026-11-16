package driver

import (
	"testing"

	"github.com/Garden12138/xcli/internal/api"
)

func TestClassifyText(t *testing.T) {
	for _, tc := range []struct {
		text string
		kind ErrorKind
	}{
		{"Not logged in. Run codex login", ErrAuth}, {"401 Unauthorized", ErrAuth},
		{"429 Too Many Requests", ErrQuota}, {"rate limit exceeded", ErrQuota}, {"quota exceeded", ErrQuota},
		{"usage_limit_exceeded", ErrQuota}, {"Workspace Trust Required", ErrTrust}, {"something else", ErrNone},
	} {
		if got := ClassifyText(tc.text, ""); got != tc.kind {
			t.Errorf("%q: %s, want %s", tc.text, got, tc.kind)
		}
	}
}

func TestSwitchable(t *testing.T) {
	for _, kind := range []api.Kind{api.KindAsk, api.KindCode, api.KindReview, api.KindImage} {
		for _, errKind := range []ErrorKind{ErrNotInstalled, ErrAuth, ErrQuota, ErrTrust} {
			if !errKind.Switchable(kind) {
				t.Errorf("%s must be switchable for %s", errKind, kind)
			}
		}
		if got := ErrNoArtifact.Switchable(kind); got != (kind == api.KindImage) {
			t.Errorf("no artifact for %s: %t", kind, got)
		}
		for _, errKind := range []ErrorKind{ErrNone, ErrPermissionDenied, ErrTimeout, ErrCancelled, ErrSchemaMismatch, ErrProtocol, ErrFailed} {
			if errKind.Switchable(kind) {
				t.Errorf("%s switched for %s", errKind, kind)
			}
		}
	}
}
