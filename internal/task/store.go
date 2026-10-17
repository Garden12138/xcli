package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	return os.Rename(f.Name(), filepath.Join(dir, name))
}

func (s *Store) Load(id string) (Task, error) {
	if !validID(id) {
		return Task{}, errors.New("invalid task id")
	}
	data, err := os.ReadFile(filepath.Join(s.Dir(id), "task.json"))
	if err != nil {
		return Task{}, err
	}
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return Task{}, fmt.Errorf("decode task: %w", err)
	}
	if t.ID != id || t.Version != 1 {
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
