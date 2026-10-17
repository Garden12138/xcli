package task

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/runstore"
)

type Store struct{ root string }

func NewStore() (*Store, error) {
	root, err := runstore.DataPath()
	if err != nil {
		return nil, err
	}
	return &Store{root: filepath.Join(root, "tasks")}, nil
}

func validID(id string) bool {
	return strings.HasPrefix(id, "task-") && filepath.Base(id) == id && !strings.ContainsAny(id, `/\\`)
}

func (s *Store) Dir(id string) string { return filepath.Join(s.root, id) }

func (s *Store) Save(t *Task) error {
	if !validID(t.ID) {
		return errors.New("invalid task id")
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	if t.Version == 2 {
		// The journal is authoritative if a crash interrupts snapshot rename.
		f, err := os.OpenFile(filepath.Join(s.Dir(t.ID), "checkpoints.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if os.IsNotExist(err) {
			if err = os.MkdirAll(s.Dir(t.ID), 0o700); err == nil {
				f, err = os.OpenFile(filepath.Join(s.Dir(t.ID), "checkpoints.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			}
		}
		if err != nil {
			return err
		}
		compact, err := json.Marshal(t)
		if err == nil {
			_, err = f.Write(append(compact, '\n'))
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return s.Write(t.ID, "task.json", append(data, '\n'))
}

func (s *Store) Write(id, name string, data []byte) error {
	if !validID(id) || filepath.Base(name) != name || name == "." || name == ".." {
		return errors.New("invalid task path")
	}
	dir := s.Dir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".task-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (s *Store) Load(id string) (Task, error) {
	if !validID(id) {
		return Task{}, errors.New("invalid task id")
	}
	data, snapshotErr := os.ReadFile(filepath.Join(s.Dir(id), "task.json"))
	var t Task
	err := json.Unmarshal(data, &t)
	lines, _, journalErr := readJournal(filepath.Join(s.Dir(id), "checkpoints.jsonl"))
	if journalErr != nil && !os.IsNotExist(journalErr) {
		return Task{}, journalErr
	}
	if len(lines) > 0 {
		err = json.Unmarshal(lines[len(lines)-1], &t)
	} else if snapshotErr != nil {
		return Task{}, snapshotErr
	}
	if err != nil {
		return Task{}, fmt.Errorf("decode task: %w", err)
	}
	if t.ID != id || (t.Version != 1 && t.Version != 2) {
		return Task{}, errors.New("invalid task identity or version")
	}
	return t, nil
}

func (s *Store) Append(e Event) error {
	if !validID(e.TaskID) {
		return errors.New("invalid task id")
	}
	f, err := os.OpenFile(filepath.Join(s.Dir(e.TaskID), "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(e)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// readJournal ignores only an unfinished final line; malformed complete records
// are errors. Writers truncate that tail under the owner lock before appending.
func readJournal(path string) ([][]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	reader := bufio.NewReader(io.LimitReader(f, 512<<20))
	var lines [][]byte
	var length int64
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 16<<20 {
			return nil, length, errors.New("task journal record exceeds 16 MiB")
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, length, err
		}
		if !json.Valid(bytes.TrimSpace(line)) {
			return nil, length, errors.New("corrupt task journal record")
		}
		lines = append(lines, line)
		length += int64(len(line))
	}
	info, err := f.Stat()
	if err == nil && info.Size() >= 512<<20 {
		return nil, length, errors.New("task journal exceeds 512 MiB")
	}
	return lines, length, err
}

func (s *Store) Recover(t *Task) error {
	for _, name := range []string{"checkpoints.jsonl", "events.jsonl"} {
		lines, length, err := readJournal(filepath.Join(s.Dir(t.ID), name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if name == "events.jsonl" {
			var previous uint64
			for _, line := range lines {
				var e Event
				if err := json.Unmarshal(line, &e); err != nil {
					return err
				}
				if e.TaskID != t.ID || e.Sequence <= previous {
					return errors.New("invalid task event identity or sequence")
				}
				previous = e.Sequence
			}
			if previous > t.Sequence {
				t.Sequence = previous
			}
		}
		if err := os.Truncate(filepath.Join(s.Dir(t.ID), name), length); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Events(id string, after uint64) ([]Event, error) {
	if !validID(id) {
		return nil, errors.New("invalid task id")
	}
	lines, _, err := readJournal(filepath.Join(s.Dir(id), "events.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Event{}
	var previous uint64
	for _, line := range lines {
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		if e.TaskID != id || e.Sequence <= previous {
			return nil, errors.New("invalid task event sequence")
		}
		previous = e.Sequence
		if e.Sequence > after {
			result = append(result, e)
		}
	}
	return result, nil
}

type Control struct {
	StopSequence uint64   `json:"stop_sequence"`
	Updates      []Update `json:"updates"`
	Next         uint64   `json:"next"`
}

func (s *Store) Control(id string) (Control, error) {
	var c Control
	data, err := os.ReadFile(filepath.Join(s.Dir(id), "control.json"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(data, &c)
	return c, err
}
func (s *Store) Queue(id string, u Update, stop bool) (Update, error) {
	t, err := s.Load(id)
	if err != nil {
		return u, err
	}
	lock, err := s.Lock(id, "control.lock", true)
	if err != nil {
		return u, err
	}
	defer lock.Close()
	c, err := s.Control(id)
	if err != nil {
		return u, err
	}
	if !stop {
		maxCalls, timeout := t.MaxCalls, t.Options.Timeout
		for _, pending := range c.Updates {
			if pending.MaxCalls > maxCalls {
				maxCalls = pending.MaxCalls
			}
			if pending.Timeout > timeout {
				timeout = pending.Timeout
			}
		}
		if u.Timeout < 0 || (u.Timeout != 0 && (u.Timeout < timeout || u.Timeout <= t.Elapsed)) {
			return u, errors.New("--timeout cannot reduce the cumulative execution budget")
		}
		if u.MaxCalls < 0 || (u.MaxCalls != 0 && (u.MaxCalls < maxCalls || u.MaxCalls < t.Calls)) {
			return u, errors.New("--max-calls cannot reduce the cumulative invocation budget")
		}
		if len(u.Message) > 128<<10 {
			return u, errors.New("Task update exceeds 128 KiB")
		}
	}
	if stop {
		c.StopSequence++
	} else {
		c.Next++
		u.Sequence = c.Next
		u.Time = time.Now().UTC()
		c.Updates = append(c.Updates, u)
	}
	data, err := json.Marshal(c)
	if err != nil {
		return u, err
	}
	return u, s.Write(id, "control.json", data)
}
