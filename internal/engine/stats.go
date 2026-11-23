package engine

import (
	"fmt"
	"os"

	"github.com/Garden12138/xcli/internal/stats"
	"github.com/Garden12138/xcli/internal/store"
)

func (e *Engine) appendUsage(taskDir string, rec store.Record, calls []stats.Call) {
	events, err := store.ReadEvents(taskDir, 0)
	if err == nil {
		if record, ok := stats.FromTask(rec, events, calls); ok {
			err = stats.Append(e.Home, record)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "xcli: 请检查统计账本和任务事件的读写权限：%q\n", err.Error())
	}
}
