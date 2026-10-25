package modelstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Log writes only typed caller-authored diagnostic records, with bounded retention.
type Log struct {
	mu    sync.Mutex
	path  string
	file  *os.File
	limit int64
}

func (s *Store) OpenLog(name string, limit int64) (*Log, error) {
	if name != "requests.jsonl" && name != "service.log" {
		return nil, errors.New("invalid gateway log name")
	}
	path := filepath.Join(s.Root, name)
	if err := checkPrivate(path, false); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Log{path: path, file: f, limit: limit}, nil
}

func (l *Log) Append(record any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return errors.New("gateway log is closed")
	}
	info, err := l.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() >= l.limit {
		if err = l.file.Close(); err != nil {
			return err
		}
		l.file = nil
		backup := l.path + ".1"
		if err = checkPrivate(backup, false); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err = os.Remove(backup); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err = os.Rename(l.path, backup); err != nil {
			return err
		}
		l.file, err = os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(l.file).Encode(record)
}

func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
