package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Garden12138/xcli/internal/api"
)

func (t *Task) Emit(typ, step, agent string, data any) (api.Event, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	event, err := api.NewEvent(t.ID(), t.seq+1, typ, step, agent, data)
	if err != nil {
		return api.Event{}, err
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return api.Event{}, err
	}
	f, err := os.OpenFile(t.Path("events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return api.Event{}, err
	}
	_, err = f.Write(append(raw, '\n'))
	err = errors.Join(err, f.Close())
	if err != nil {
		return api.Event{}, err
	}
	t.seq = event.Seq
	return event, nil
}

func ReadEvents(taskDir string, after int) ([]api.Event, error) {
	f, err := os.Open(filepath.Join(taskDir, "events.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return []api.Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	events := []api.Event{}
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return events, nil // 等下一次读取完整的追加行。
		}
		if err != nil {
			return nil, err
		}
		var e api.Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		if e.Seq > after {
			events = append(events, e)
		}
	}
}

func Follow(ctx context.Context, taskDir string, after int, fn func(api.Event) error) error {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		events, err := ReadEvents(taskDir, 0)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.Seq > after {
				if err := fn(e); err != nil {
					return err
				}
				after = e.Seq
			}
			if e.Type == "task.finished" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
