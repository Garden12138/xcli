//go:build unix

package stats

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func ledgerPath(home string) string {
	return filepath.Join(home, "stats", "usage.jsonl")
}

func Append(home string, r Record) (err error) {
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	path := ledgerPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func ReadLedger(path string) (records []Record, skipped int, err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return records, skipped, err
		}
		if len(line) > 0 {
			var r Record
			if json.Unmarshal(line, &r) != nil || r.V != 1 || r.TaskID == "" {
				skipped++
			} else {
				records = append(records, r)
			}
		}
		if err == io.EOF {
			return records, skipped, nil
		}
	}
}
