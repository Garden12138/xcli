package store

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/api"
)

type Record struct {
	ID        string           `json:"id"`
	Status    api.Status       `json:"status"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	PID       int              `json:"pid,omitempty"`
	Request   api.Request      `json:"request"`
	Plan      *api.Plan        `json:"plan,omitempty"`
	Steps     []api.StepResult `json:"steps,omitempty"`
	Result    *api.Result      `json:"result,omitempty"`
}

type Store struct{ Root string }
type Task struct {
	dir string
	mu  sync.Mutex
	rec Record
	seq int
}

var taskIDPattern = regexp.MustCompile(`^t-\d{8}-\d{6}-[0-9a-f]{4}$`)
var stepIDPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

func Open(home string) (*Store, error) {
	root, err := filepath.Abs(filepath.Join(home, "tasks"))
	if err == nil {
		err = os.MkdirAll(root, 0700)
	}
	return &Store{Root: root}, err
}

func NewID(now time.Time) string {
	var suffix [2]byte
	_, _ = rand.Read(suffix[:])
	return fmt.Sprintf("t-%s-%x", now.Format("20060102-150405"), suffix)
}

func (s *Store) Create(req api.Request) (*Task, error) {
	now := time.Now().UTC()
	for range 10 {
		id := NewID(now)
		dir := filepath.Join(s.Root, id)
		if err := os.Mkdir(dir, 0700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return nil, err
		}
		t := &Task{dir: dir, rec: Record{ID: id, Status: api.StatusQueued, CreatedAt: now, UpdatedAt: now, Request: req}}
		if err := t.Update(func(*Record) {}); err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
		return t, nil
	}
	return nil, errors.New("请重试创建任务以生成新的任务 ID")
}

func (s *Store) Load(id string) (*Task, error) {
	if !taskIDPattern.MatchString(id) {
		return nil, errors.New("请提供 xcli 返回的完整任务 ID")
	}
	t := &Task{dir: filepath.Join(s.Root, id)}
	raw, err := os.ReadFile(t.Path("task.json"))
	if err != nil {
		return nil, fmt.Errorf("请用 xcli list 检查任务 %s：%w", id, err)
	}
	if err := json.Unmarshal(raw, &t.rec); err != nil {
		return nil, fmt.Errorf("请检查任务 %s 的 task.json：%w", id, err)
	}
	if t.rec.ID != id {
		return nil, errors.New("请检查 task.json 中的 ID 与任务目录是否一致")
	}
	events, err := ReadEvents(t.dir, 0)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if event.Seq > t.seq {
			t.seq = event.Seq
		}
	}
	return t, nil
}

func (s *Store) List(limit int) ([]Record, error) {
	dirs, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, err
	}
	rows := []Record{}
	for _, entry := range dirs {
		if !entry.IsDir() || !taskIDPattern.MatchString(entry.Name()) {
			continue
		}
		t, err := s.Load(entry.Name())
		if err != nil {
			return nil, err
		}
		rows = append(rows, t.Snapshot())
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func (s *Store) Remove(id string) error {
	if !taskIDPattern.MatchString(id) {
		return errors.New("请提供 xcli 返回的完整任务 ID")
	}
	return os.RemoveAll(filepath.Join(s.Root, id))
}

func (t *Task) ID() string  { return filepath.Base(t.dir) }
func (t *Task) Dir() string { return t.dir }
func (t *Task) Path(parts ...string) string {
	return filepath.Join(append([]string{t.dir}, parts...)...)
}

func clone(rec Record) Record {
	raw, _ := json.Marshal(rec)
	var out Record
	_ = json.Unmarshal(raw, &out)
	return out
}

func (t *Task) Snapshot() Record {
	t.mu.Lock()
	defer t.mu.Unlock()
	return clone(t.rec)
}

func (t *Task) Update(fn func(*Record)) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	next := clone(t.rec)
	fn(&next)
	next.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	tmp := t.Path(".task.json.tmp")
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, t.Path("task.json")); err != nil {
		return err
	}
	t.rec = clone(next)
	return nil
}

func (t *Task) StepDir(stepID string, round int) (string, error) {
	if !stepIDPattern.MatchString(stepID) || round < 1 {
		return "", errors.New("请使用 1–32 位小写字母、数字、下划线或连字符的步骤 ID，并将 round 设为正整数")
	}
	dir := t.Path("steps", fmt.Sprintf("%s-r%d", stepID, round))
	return dir, os.MkdirAll(dir, 0700)
}
