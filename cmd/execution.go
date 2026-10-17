package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/runstore"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
)

func openRawOutput(store *runstore.Store, id string, record bool) (*os.File, error) {
	if !record {
		return nil, nil
	}
	return store.OpenOutput(id, "output.log")
}

func applyResult(record *runstore.Record, result agent.RunResult) {
	record.RetryNotices = result.RetryNotices
	record.ExitCode = result.ExitCode
	record.Status = result.Status
	record.ParseStatus = result.ParseStatus
	record.ErrorKind = result.ErrorKind
	record.Error = result.Error
	record.SessionID = result.SessionID
	record.Usage = result.Usage
}

// Human activity is rate limited, including for non-TTY consumers. JSONL keeps
// the bounded runtime stream and never mixes the final answer with prose.
func eventSink(writer io.Writer, asEvents bool) func(agent.Event) error {
	encoder := json.NewEncoder(writer)
	var last time.Time
	return func(event agent.Event) error {
		if asEvents {
			return encoder.Encode(event)
		}
		if event.Type == "progress" && time.Since(last) < 250*time.Millisecond {
			return nil
		}
		last = time.Now()
		_, err := fmt.Fprintf(writer, "[%s] %s %s\n", event.RunID, event.Type, event.Message)
		return err
	}
}

func processCLIExit(process xruntime.ProcessResult) int {
	_, code := agent.Normalize(agent.RunResult{ParseStatus: "not_applicable"}, process.ExitCode, process.Canceled, process.TimedOut, nil)
	if process.IOError && code == 0 {
		return 1
	}
	return code
}
