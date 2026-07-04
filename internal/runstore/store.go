package runstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Record struct {
	ID              string       `json:"id"`
	Kind            string       `json:"kind"`
	Agent           string       `json:"agent,omitempty"`
	SelectionSource string       `json:"selection_source,omitempty"`
	RouteRule       string       `json:"route_rule,omitempty"`
	Workflow        string       `json:"workflow,omitempty"`
	MaxParallel     int          `json:"max_parallel,omitempty"`
	Cwd             string       `json:"cwd"`
	StartedAt       time.Time    `json:"started_at"`
	EndedAt         time.Time    `json:"ended_at"`
	Status          string       `json:"status"`
	ExitCode        int          `json:"exit_code"`
	SessionID       string       `json:"session_id,omitempty"`
	OutputFile      string       `json:"output_file,omitempty"`
	Steps           []StepRecord `json:"steps,omitempty"`
}

type StepRecord struct {
	ID         string    `json:"id"`
	Agent      string    `json:"agent"`
	Status     string    `json:"status"`
	ExitCode   int       `json:"exit_code"`
	SessionID  string    `json:"session_id,omitempty"`
	OutputFile string    `json:"output_file,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	EndedAt    time.Time `json:"ended_at,omitempty"`
	Attempts   int       `json:"attempts,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type Store struct {
	root string
}

func New() (*Store, error) {
	root, err := DataPath()
	if err != nil {
		return nil, err
	}
	return &Store{root: filepath.Join(root, "runs")}, nil
}

func DataPath() (string, error) {
	if root := os.Getenv("XDG_DATA_HOME"); root != "" {
		return filepath.Join(root, "xcli"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "xcli"), nil
}

func NewID(prefix string) string {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("%s-%s", prefix, time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().UTC().Format("20060102T150405Z"), hex.EncodeToString(random))
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) Save(record Record) error {
	if record.ID == "" || strings.ContainsAny(record.ID, `/\\`) {
		return errors.New("invalid run id")
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create run directory: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run record: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomic(filepath.Join(s.root, record.ID+".json"), data); err != nil {
		return fmt.Errorf("save run record: %w", err)
	}
	return nil
}

func (s *Store) SaveOutput(runID, name string, data []byte) (string, error) {
	if runID == "" || strings.ContainsAny(runID, `/\\`) || name == "" || filepath.Base(name) != name {
		return "", errors.New("invalid output path")
	}
	directory := filepath.Join(s.root, runID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	path := filepath.Join(directory, name)
	if err := writeAtomic(path, data); err != nil {
		return "", fmt.Errorf("save output: %w", err)
	}
	return path, nil
}

func (s *Store) Load(id string) (Record, error) {
	if id == "" || strings.ContainsAny(id, `/\\`) {
		return Record{}, errors.New("invalid run id")
	}
	data, err := os.ReadFile(filepath.Join(s.root, id+".json"))
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("decode run record: %w", err)
	}
	return record, nil
}

func (s *Store) List() ([]Record, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		record, err := s.Load(id)
		if err != nil {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt.After(records[j].StartedAt)
	})
	return records, nil
}

func writeAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	tmp, err := os.CreateTemp(directory, ".xcli-run-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
