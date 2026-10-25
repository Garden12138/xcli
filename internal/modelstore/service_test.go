package modelstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceLockStopAndStaleInstanceIsolation(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.AcquireService("127.0.0.1:18741", "digest", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcquireService("127.0.0.1:18742", "other", "test"); !errors.Is(err, ErrLocked) {
		t.Fatal("duplicate service", err)
	}
	state, err := s.Status()
	if err != nil || !state.Active || state.InstanceID != service.State.InstanceID {
		t.Fatal(state, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closed := make(chan error, 1)
	go func() { err := service.WatchStop(ctx, cancel); closed <- errors.Join(err, service.Close()) }()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err = s.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if err = <-closed; err != nil {
		t.Fatal(err)
	}
	next, err := s.AcquireService("127.0.0.1:18743", "digest", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if next.State.InstanceID == service.State.InstanceID {
		t.Fatal("instance nonce reused")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 180*time.Millisecond)
	defer cancel2()
	if err = next.WatchStop(ctx2, cancel2); err != nil {
		t.Fatal(err)
	}
	if time.Until(deadlineOf(ctx2)) > 20*time.Millisecond {
		t.Fatal("stale stop terminated new instance")
	}
	if err = next.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.Stop(context.Background()); err != nil {
		t.Fatal("idempotent stop", err)
	}
}

func deadlineOf(ctx context.Context) time.Time { d, _ := ctx.Deadline(); return d }

func TestUnsupportedServiceStateIsNeverOverwritten(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Root, "service.json")
	before := []byte(`{"schema":99,"pid":1}`)
	if err = WritePrivate(path, before); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcquireService("127.0.0.1:18741", "digest", "test"); err == nil {
		t.Fatal("unknown state migrated")
	}
	if _, err = s.Status(); err == nil {
		t.Fatal("unknown state accepted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("unknown state changed")
	}
}

func TestBoundedDiagnosticLogRotation(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	log, err := s.OpenLog("requests.jsonl", 64)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err = log.Append(map[string]any{"status": 200, "request_id": "fixture"}); err != nil {
			t.Fatal(err)
		}
	}
	if err = log.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"requests.jsonl", "requests.jsonl.1"} {
		info, err := os.Stat(filepath.Join(s.Root, name))
		if err != nil || info.Size() > 128 {
			t.Fatal(name, info, err)
		}
	}
	if _, err = s.OpenLog("../escape", 64); err == nil {
		t.Fatal("log path escaped")
	}
}
