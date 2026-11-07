package agentstore

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Garden12138/xcli/internal/modelstore"
	"os"
	"path/filepath"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
)

func TestJournalRecoveryAndSingleWriter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	s, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	var instance string
	_ = s.View(func(st State) error { instance = st.InstanceID; return nil })
	if _, e = Open(root); e == nil {
		t.Fatal("second writer accepted")
	}
	if e = s.Update(func(st *State) error { st.Counters["test"] = Counter{Dispatches: 1}; return nil }); e != nil {
		t.Fatal(e)
	}
	if e = s.Checkpoint(); e != nil {
		t.Fatal(e)
	}
	_ = s.Close()
	// Checkpoint is a cache; recovery authority remains the journal.
	if e = os.WriteFile(filepath.Join(root, "checkpoint.json"), []byte("broken"), 0o600); e != nil {
		t.Fatal(e)
	}
	f, _ := os.OpenFile(filepath.Join(root, "journal"), os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.Write([]byte("NAG1 partial"))
	f.Close()
	s, e = Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	_ = s.View(func(st State) error {
		if st.InstanceID != instance || st.Counters["test"].Dispatches != 1 {
			t.Fatal(st)
		}
		return nil
	})
}
func TestJournalCommittedCorruptionFailsClosed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	s, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	p := filepath.Join(root, "journal")
	b, _ := os.ReadFile(p)
	b[90] ^= 1
	_ = os.WriteFile(p, b, 0o600)
	if _, e = Open(root); !errors.Is(e, ErrCorrupt) {
		t.Fatalf("%v", e)
	}
}
func TestAmbiguousSyncPoisonsWriter(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "state"))
	if e != nil {
		t.Fatal(e)
	}
	s.BeforeSync = func() error { return errors.New("injected fsync failure") }
	if e = s.Update(func(st *State) error { st.Counters["dispatch"] = Counter{Dispatches: 1}; return nil }); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	if e = s.Update(func(st *State) error { return nil }); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	root := s.Root
	s.Close()
	s, e = Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	_ = s.View(func(st State) error {
		if st.Counters["dispatch"].Dispatches != 1 {
			t.Fatal("ambiguous commit not recovered")
		}
		return nil
	})
}
func TestTokenIsolationRotationAndRevocation(t *testing.T) {
	root, e := Prepare(filepath.Join(t.TempDir(), "state"))
	if e != nil {
		t.Fatal(e)
	}
	c := &Clients{Root: root}
	grant := Client{ID: "worklog", Scopes: []string{"task:read"}, Agents: []string{"fake"}, Operations: []string{"text_task"}, Policies: []string{"fake"}}
	token, e := c.Issue(grant, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Authenticate(token); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(root, "clients.json"))
	if string(b) == token || api.Hash(b) == "" {
		t.Fatal("token serialization")
	}
	token2, e := c.Issue(grant, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Authenticate(token); e == nil {
		t.Fatal("old token works")
	}
	if _, e = c.Authenticate(token2); e != nil {
		t.Fatal(e)
	}
	if e = c.Revoke("worklog"); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Authenticate(token2); e == nil {
		t.Fatal("revoked token works")
	}
}
func TestIdleReadAndNoopDoNotGrowJournal(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "state"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	before, _ := s.file.Stat()
	for range 100 {
		if e = s.Update(func(*State) error { return nil }); e != nil {
			t.Fatal(e)
		}
	}
	after, _ := s.file.Stat()
	if before.Size() != after.Size() {
		t.Fatal("idle service fills journal")
	}
}
func TestServiceGenerationStopsWithoutPIDAndIdentityPersists(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "state"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	state, e := s.Publish("127.0.0.1:18742", "config")
	if e != nil {
		t.Fatal(e)
	}
	current, e := Status(s.Root)
	if e != nil || !current.Active || current.InstanceID != state.InstanceID {
		t.Fatal(current, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- WatchStop(ctx, s.Root, state, cancel) }()
	b, _ := json.Marshal(map[string]string{"instance_id": state.InstanceID, "generation_id": "stale-generation"})
	if e = modelstore.WritePrivate(filepath.Join(s.Root, "stop.json"), b); e != nil {
		t.Fatal(e)
	}
	select {
	case <-ctx.Done():
		t.Fatal("stale stop canceled current worker")
	case <-time.After(150 * time.Millisecond):
	}
	b, _ = json.Marshal(map[string]string{"instance_id": state.InstanceID, "generation_id": state.GenerationID})
	if e = modelstore.WritePrivate(filepath.Join(s.Root, "stop.json"), b); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("stop not observed")
	}
	if e = s.Unpublish(state); e != nil {
		t.Fatal(e)
	}
}
func TestStatusProbeCannotRejectAStartingWriter(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "state"))
	if e != nil {
		t.Fatal(e)
	}
	root := s.Root
	if _, e = s.Publish("127.0.0.1:18742", "config"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	for range 20 {
		done := make(chan error, 1)
		go func() { _, e := Status(root); done <- e }()
		next, e := Open(root)
		if e != nil {
			t.Fatal(e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
		next.Close()
	}
}
func TestModelGatewayStateRootCannotBeReused(t *testing.T) {
	root := filepath.Join(t.TempDir(), "model")
	if e := os.Mkdir(root, 0o700); e != nil {
		t.Fatal(e)
	}
	original := []byte(`{"version":1,"clients":{}}`)
	if e := os.WriteFile(filepath.Join(root, "clients.json"), original, 0o600); e != nil {
		t.Fatal(e)
	}
	if _, e := Prepare(root); e == nil {
		t.Fatal("Model state root accepted")
	}
	data, _ := os.ReadFile(filepath.Join(root, "clients.json"))
	if string(data) != string(original) {
		t.Fatal("Model store changed")
	}
}
