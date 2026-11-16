//go:build unix

package proc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireSlotExclusive(t *testing.T) {
	dir := t.TempDir()
	release, err := AcquireSlot(context.Background(), dir, "codex", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var waits atomic.Int32
	acquired := make(chan error, 1)
	go func() {
		free, err := AcquireSlot(ctx, dir, "codex", 1, func() { waits.Add(1) })
		if err == nil {
			free()
		}
		acquired <- err
	}()
	deadline := time.Now().Add(time.Second)
	for waits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if waits.Load() == 0 {
		t.Fatal("second acquisition did not wait")
	}
	release()
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("slot was not released")
	}
}

func TestAcquireSlotCancel(t *testing.T) {
	dir := t.TempDir()
	free, err := AcquireSlot(context.Background(), dir, "cursor", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer free()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := AcquireSlot(ctx, dir, "cursor", 1, nil); err == nil {
		t.Fatal("waiting slot ignored cancellation")
	}
}

func TestSlotHelperProcess(t *testing.T) {
	dir := os.Getenv("XCLI_TEST_SLOT_DIR")
	if dir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	free, err := AcquireSlot(ctx, dir, "codex", 1, func() { _ = os.WriteFile(filepath.Join(dir, "waiting"), nil, 0600) })
	if err != nil {
		t.Fatal(err)
	}
	defer free()
	if err := os.WriteFile(filepath.Join(dir, "acquired"), nil, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireSlotAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	free, err := AcquireSlot(context.Background(), dir, "codex", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer free()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSlotHelperProcess$")
	cmd.Env = append(os.Environ(), "XCLI_TEST_SLOT_DIR="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer cmd.Process.Kill()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "waiting")); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, "waiting")); err != nil {
		t.Fatal("child process did not wait for slot")
	}
	if _, err := os.Stat(filepath.Join(dir, "acquired")); err == nil {
		t.Fatal("child acquired occupied slot")
	}
	free()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("child did not acquire released slot")
	}
	if _, err := os.Stat(filepath.Join(dir, "acquired")); err != nil {
		t.Fatal(err)
	}
}
