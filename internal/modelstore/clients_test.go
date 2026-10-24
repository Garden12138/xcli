package modelstore

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Garden12138/xcli/internal/model"
)

func TestClientTokenLifecycleAndPrivateStorage(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	c := model.Client{ID: "worklog", AllowedModels: []string{"text"}, AllowRemote: true, RequestsPerMinute: 60}
	token, err := s.Issue(c, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) != 32 {
		t.Fatal("token entropy", err)
	}
	if got, err := s.Authenticate(token); err != nil || got.ID != c.ID {
		t.Fatal(got, err)
	}
	data, err := os.ReadFile(filepath.Join(s.Root, "clients.json"))
	if err != nil || strings.Contains(string(data), token) {
		t.Fatal("plaintext token retained", err)
	}
	if _, err = s.Issue(c, false); err == nil {
		t.Fatal("existing client overwritten")
	}
	newToken, err := s.Issue(c, true)
	if err != nil || newToken == token {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(token); err == nil {
		t.Fatal("old token still valid")
	}
	if err = s.Revoke(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(newToken); err == nil {
		t.Fatal("revoked token valid")
	}
	if err = os.WriteFile(filepath.Join(s.Root, "clients.json"), []byte(`{"version":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Clients(); err == nil {
		t.Fatal("corrupt store accepted")
	}
}

func TestClientMutationsDoNotLoseConcurrentUpdates(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"a", "b", "c", "d"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Issue(model.Client{ID: id, AllowedModels: []string{"text"}, RequestsPerMinute: 10}, false)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	clients, err := s.Clients()
	if err != nil || len(clients) != 4 {
		t.Fatal(clients, err)
	}
}

func TestSecretReferencesAreExplicitAndPrivate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "key")
	if err := os.WriteFile(path, []byte("fixture-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if v, err := ResolveSecret("file:key", root); v != "fixture-secret" || err != nil {
		t.Fatal(v, err)
	}
	t.Setenv("XCLI_TEST_KEY", "fixture-env")
	if v, err := ResolveSecret("env:XCLI_TEST_KEY", root); v != "fixture-env" || err != nil {
		t.Fatal(v, err)
	}
	if err := os.Symlink(path, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveSecret("file:link", root); err == nil {
		t.Fatal("symlink secret accepted")
	}
	if _, err := ResolveSecret("keychain:unsupported", root); err == nil {
		t.Fatal("unsupported store accepted")
	}
}
