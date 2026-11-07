//go:build darwin || linux

package runtime

import (
	"context"
	"github.com/Garden12138/xcli/internal/agent"
	"testing"
	"time"
)

func TestGatewayBoundedCleanupWithInheritedPipe(t *testing.T) {
	start := time.Now()
	r, e := RunProcess(context.Background(), agent.CommandSpec{Command: "/bin/sh", Args: []string{"-c", "sleep 1.5 & exit 0"}}, ProcessOptions{Env: []string{"PATH=/usr/bin:/bin"}, SeparateProcess: true, BoundedCleanup: true, CaptureStdout: true})
	if e != nil || time.Since(start) > time.Second || r.StopConfirmed || !r.IOError {
		t.Fatal(r, e, time.Since(start))
	}
}
func TestGatewayBoundedCleanupIgnoresTERM(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, e := RunProcess(ctx, agent.CommandSpec{Command: "/bin/sh", Args: []string{"-c", "trap '' TERM; while :; do sleep 1; done"}}, ProcessOptions{Env: []string{"PATH=/usr/bin:/bin"}, SeparateProcess: true, BoundedCleanup: true})
	if e != nil || !r.TimedOut || time.Since(start) > 5*time.Second+150*time.Millisecond {
		t.Fatal(r, e, time.Since(start))
	}
}
