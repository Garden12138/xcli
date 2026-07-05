package jobs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/runstore"
)

const (
	LogName  = "job.log"
	LockName = "job.lock"
)

type View struct {
	ID        string       `json:"id"`
	Agent     string       `json:"agent"`
	Status    string       `json:"status"`
	PID       int          `json:"pid"`
	Cwd       string       `json:"cwd"`
	StartedAt time.Time    `json:"started_at"`
	EndedAt   *time.Time   `json:"ended_at,omitempty"`
	ExitCode  *int         `json:"exit_code,omitempty"`
	SessionID string       `json:"session_id,omitempty"`
	Usage     *agent.Usage `json:"usage,omitempty"`
	LogFile   string       `json:"log_file,omitempty"`
}

type Files struct {
	Directory string
	LogPath   string
	LockPath  string
	Log       *os.File
	Lock      *os.File
}

type Manager struct {
	Store *runstore.Store
	Now   func() time.Time
}

func IsTerminal(status string) bool {
	switch status {
	case "success", "failed", "canceled", "timed_out", "killed", "orphaned":
		return true
	default:
		return false
	}
}

func ToView(record runstore.Record) View {
	view := View{
		ID: record.ID, Agent: record.Agent, Status: record.Status, PID: record.PID,
		Cwd: record.Cwd, StartedAt: record.StartedAt, SessionID: record.SessionID,
		Usage: record.Usage, LogFile: record.LogFile,
	}
	if IsTerminal(record.Status) {
		endedAt := record.EndedAt
		exitCode := record.ExitCode
		view.EndedAt = &endedAt
		view.ExitCode = &exitCode
	}
	return view
}

func (m Manager) CreateFiles(id string) (Files, error) {
	directory, err := jobDirectory(m.Store, id)
	if err != nil {
		return Files{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Files{}, fmt.Errorf("create job directory: %w", err)
	}
	logPath := filepath.Join(directory, LogName)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return Files{}, fmt.Errorf("create job log: %w", err)
	}
	lockPath := filepath.Join(directory, LockName)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		logFile.Close()
		return Files{}, fmt.Errorf("create job lock: %w", err)
	}
	if err := lockExclusive(lockFile); err != nil {
		lockFile.Close()
		logFile.Close()
		return Files{}, fmt.Errorf("lock job: %w", err)
	}
	return Files{
		Directory: directory, LogPath: logPath, LockPath: lockPath,
		Log: logFile, Lock: lockFile,
	}, nil
}

func (m Manager) Load(id string) (runstore.Record, error) {
	record, err := m.Store.Load(id)
	if err != nil {
		return runstore.Record{}, err
	}
	if !record.Background {
		return runstore.Record{}, fmt.Errorf("run %q is not a background job", id)
	}
	return m.Reconcile(record)
}

func (m Manager) List() ([]runstore.Record, error) {
	records, err := m.Store.List()
	if err != nil {
		return nil, err
	}
	result := make([]runstore.Record, 0)
	for _, record := range records {
		if !record.Background {
			continue
		}
		record, err = m.Reconcile(record)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (m Manager) Reconcile(record runstore.Record) (runstore.Record, error) {
	if !record.Background || IsTerminal(record.Status) {
		return record, nil
	}
	lockPath, err := artifactPath(m.Store, record.ID, LockName)
	if err != nil {
		return runstore.Record{}, err
	}
	active, err := lockHeld(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return runstore.Record{}, fmt.Errorf("inspect job lock: %w", err)
	}
	if active {
		return record, nil
	}
	record.Status = "orphaned"
	record.ExitCode = 1
	record.EndedAt = m.now().UTC()
	if err := m.Store.Save(record); err != nil {
		return runstore.Record{}, err
	}
	return record, nil
}

func (m Manager) LogPath(id string) (string, error) {
	return artifactPath(m.Store, id, LogName)
}

func (m Manager) LockHeld(id string) (bool, error) {
	path, err := artifactPath(m.Store, id, LockName)
	if err != nil {
		return false, err
	}
	return lockHeld(path)
}

func (m Manager) MarkKilled(record runstore.Record) (runstore.Record, error) {
	record.Status = "killed"
	record.ExitCode = 137
	record.EndedAt = m.now().UTC()
	if err := m.Store.Save(record); err != nil {
		return runstore.Record{}, err
	}
	return record, nil
}

func (m Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func jobDirectory(store *runstore.Store, id string) (string, error) {
	if id == "" || filepath.Base(id) != id || id == "." || id == ".." {
		return "", errors.New("invalid job id")
	}
	return filepath.Join(store.Root(), id), nil
}

func artifactPath(store *runstore.Store, id, name string) (string, error) {
	directory, err := jobDirectory(store, id)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, name), nil
}
