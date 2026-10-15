package runtime

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
)

type Outcome struct {
	Process     ProcessResult
	Result      agent.RunResult
	CLIExitCode int
}

// Execute is shared by foreground, background, resume and workflow attempts.
// Its bounded event queue may coalesce progress; terminal events are retained.
func Execute(ctx context.Context, definition agent.Definition, spec agent.CommandSpec, options ProcessOptions, runID, stepID string, attempt int, sink func(agent.Event) error, fallbackSession ...string) (Outcome, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var queue chan agent.Event
	var done chan struct{}
	var sinkErr error
	var mu sync.Mutex
	if sink != nil {
		queue = make(chan agent.Event, 64)
		done = make(chan struct{})
		go func() {
			defer close(done)
			for event := range queue {
				mu.Lock()
				failed := sinkErr != nil
				mu.Unlock()
				if failed {
					continue
				}
				if err := sink(event); err != nil {
					mu.Lock()
					sinkErr = err
					mu.Unlock()
					cancel()
				}
			}
		}()
	}
	emit := func(kind, message string, result any) {
		if queue == nil {
			return
		}
		event := agent.Event{Version: 1, RunID: runID, StepID: stepID, Attempt: attempt, Time: time.Now().UTC(), Type: kind, Message: message, Result: result}
		if kind == "progress" {
			select {
			case queue <- event:
			default:
			}
		} else {
			queue <- event
		}
	}
	decoder := agent.NewStreamDecoder(definition.Config.Adapter, definition.Config.Output, func(message string) { emit("progress", message, nil) })
	options.Stdout = decoder
	options.CaptureStdout = false
	diagnostics := agent.NewStderrDiagnostics(definition.Config.Adapter)
	if options.Stderr == nil {
		options.Stderr = diagnostics
	} else {
		options.Stderr = io.MultiWriter(diagnostics, options.Stderr)
	}
	emit("started", "", nil)
	process, runErr := RunProcess(ctx, spec, options)
	parsed := diagnostics.Apply(decoder.Finish())
	if parsed.SessionID == "" && len(fallbackSession) > 0 {
		parsed.SessionID = fallbackSession[0]
	}
	result, cliExit := agent.Normalize(parsed, process.ExitCode, process.Canceled, process.TimedOut, runErr)
	if process.IOError && !process.Canceled && !process.TimedOut && process.ExitCode == 0 {
		result.Status = "failed"
		result.ErrorKind = "process_failed"
		result.Error = "native process I/O failed"
		cliExit = 1
	}
	result.RunID = runID
	result.Agent = definition.Name
	kind := "completed"
	if cliExit != 0 {
		kind = "failed"
	}
	emit(kind, "", result)
	if queue != nil {
		close(queue)
		<-done
	}
	mu.Lock()
	err := sinkErr
	mu.Unlock()
	if err != nil {
		result.Status = "failed"
		result.ErrorKind = "process_failed"
		result.Error = "event delivery failed"
		cliExit = 1
		return Outcome{Process: process, Result: result, CLIExitCode: cliExit}, err
	}
	return Outcome{Process: process, Result: result, CLIExitCode: cliExit}, runErr
}
