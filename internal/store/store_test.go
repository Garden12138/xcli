package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
)

func newTestTask(t *testing.T) (*Store, *Task) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Create(api.Request{Goal: "test", Metadata: map[string]string{"app": "test"}})
	if err != nil {
		t.Fatal(err)
	}
	return s, task
}

func TestCreateLoadRoundTrip(t *testing.T) {
	s, task := newTestTask(t)
	got, err := s.Load(task.ID())
	if err != nil || got.Snapshot().Request.Goal != "test" || got.Snapshot().Status != api.StatusQueued {
		t.Fatalf("load: %v, %v", got, err)
	}
	for path, mode := range map[string]os.FileMode{task.Dir(): 0700, task.Path("task.json"): 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("permissions %s: %v, %v", path, info, err)
		}
	}
	copy := task.Snapshot()
	copy.Request.Metadata["app"] = "changed"
	if task.Snapshot().Request.Metadata["app"] != "test" {
		t.Fatal("snapshot aliases task state")
	}
}

func TestUpdateIsAtomic(t *testing.T) {
	s, task := newTestTask(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for ctx.Err() == nil {
			data, err := os.ReadFile(task.Path("task.json"))
			if err != nil || !json.Valid(data) {
				t.Errorf("atomic read: %q, %v", data, err)
				return
			}
		}
	}()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			if err := task.Update(func(r *Record) { r.Steps = append(r.Steps, api.StepResult{ID: fmt.Sprint(i)}) }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	cancel()
	<-readerDone
	loaded, err := s.Load(task.ID())
	if err != nil || len(loaded.Snapshot().Steps) != 20 {
		t.Fatalf("updates: %v, %v", loaded, err)
	}
}

func TestEmitSeqAndRead(t *testing.T) {
	s, task := newTestTask(t)
	for range 3 {
		if _, err := task.Emit("notice", "", "", map[string]string{"text": "hi"}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := ReadEvents(task.Dir(), 1)
	if err != nil || len(events) != 2 || events[0].Seq != 2 || events[1].Seq != 3 {
		t.Fatalf("events: %+v, %v", events, err)
	}
	loaded, err := s.Load(task.ID())
	if err != nil {
		t.Fatal(err)
	}
	e, err := loaded.Emit("notice", "", "", nil)
	if err != nil || e.Seq != 4 {
		t.Fatalf("resumed seq: %+v, %v", e, err)
	}
	if _, err := loaded.Emit("notice", "", "", make(chan int)); err == nil {
		t.Fatal("invalid event must report serialization error")
	}
	e, err = loaded.Emit("notice", "", "", nil)
	if err != nil || e.Seq != 5 {
		t.Fatal("failed payload consumed seq")
	}
}

func TestFollowStopsOnFinished(t *testing.T) {
	_, task := newTestTask(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		for _, typ := range []string{"notice", "notice", "task.finished"} {
			time.Sleep(50 * time.Millisecond)
			if _, err := task.Emit(typ, "", "", nil); err != nil {
				t.Error(err)
			}
		}
	}()
	var seqs []int
	if err := Follow(ctx, task.Dir(), 0, func(e api.Event) error { seqs = append(seqs, e.Seq); return nil }); err != nil || len(seqs) != 3 {
		t.Fatalf("follow: %v, %v", seqs, err)
	}
	if err := Follow(ctx, task.Dir(), 3, func(api.Event) error { t.Error("unexpected event"); return nil }); err != nil {
		t.Fatalf("finished cursor: %v", err)
	}
}

func TestListNewestFirst(t *testing.T) {
	s, first := newTestTask(t)
	time.Sleep(time.Millisecond)
	second, err := s.Create(api.Request{Goal: "second"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.List(1)
	if err != nil || len(rows) != 1 || rows[0].ID != second.ID() {
		t.Fatalf("list: %+v, %v", rows, err)
	}
	if err := s.Remove(first.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(first.ID()); err == nil {
		t.Fatal("removed task still present")
	}
}

func TestNewIDFormat(t *testing.T) {
	id := NewID(time.Date(2026, 11, 13, 17, 1, 2, 0, time.UTC))
	if !regexp.MustCompile(`^t-20261113-170102-[0-9a-f]{4}$`).MatchString(id) {
		t.Fatal(id)
	}
}

func TestPathValidationAndPartialEvent(t *testing.T) {
	s, task := newTestTask(t)
	if _, err := s.Load("../outside"); err == nil {
		t.Fatal("invalid task path accepted")
	}
	if _, err := task.StepDir("../outside", 1); err == nil {
		t.Fatal("invalid step path accepted")
	}
	dir, err := task.StepDir("main", 2)
	if err != nil || dir != filepath.Join(task.Dir(), "steps", "main-r2") {
		t.Fatalf("step directory: %s, %v", dir, err)
	}
	if err := os.WriteFile(task.Path("events.jsonl"), []byte(`{"v":1`), 0600); err != nil {
		t.Fatal(err)
	}
	if rows, err := ReadEvents(task.Dir(), 0); err != nil || len(rows) != 0 {
		t.Fatalf("incomplete trailing event: %+v, %v", rows, err)
	}
}
