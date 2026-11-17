//go:build unix

package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Garden12138/xcli/internal/driver"
)

type Entry struct {
	Health         driver.Health `json:"health"`
	CheckedAt      time.Time     `json:"checked_at"`
	CooldownUntil  time.Time     `json:"cooldown_until,omitempty"`
	CooldownReason string        `json:"cooldown_reason,omitempty"`
}

type Cache struct {
	file string
	mu   sync.Mutex
}

func Open(home string) *Cache { return &Cache{file: filepath.Join(home, "cache", "health.json")} }

func (c *Cache) locked(save bool, fn func(map[string]Entry)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(c.file), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(c.file+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	entries := map[string]Entry{}
	raw, err := os.ReadFile(c.file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return fmt.Errorf("请删除健康缓存 %s 后重试：%w", c.file, err)
		}
	}
	if entries == nil {
		entries = map[string]Entry{}
	}
	fn(entries)
	if !save {
		return nil
	}
	raw, err = json.Marshal(entries)
	if err != nil {
		return err
	}
	tmp := c.file + ".tmp"
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, c.file)
}

func (c *Cache) Entry(name string) Entry {
	var entry Entry
	_ = c.locked(false, func(entries map[string]Entry) { entry = entries[name] })
	return entry
}

func (c *Cache) Available(name string, now time.Time) bool {
	return !c.Entry(name).CooldownUntil.After(now)
}

func (c *Cache) Probe(ctx context.Context, d driver.Driver, exe string, env []string, maxAge time.Duration) Entry {
	entry := c.Entry(d.Name())
	if maxAge > 0 && !entry.CheckedAt.IsZero() && time.Since(entry.CheckedAt) < maxAge && entry.Health.Path == exe {
		return entry
	}
	h := d.Probe(ctx, exe, env)
	h.Name, h.Path = d.Name(), exe
	checkedAt := time.Now().UTC()
	entry.Health, entry.CheckedAt = h, checkedAt
	_ = c.locked(true, func(entries map[string]Entry) {
		entry = entries[d.Name()]
		entry.Health, entry.CheckedAt = h, checkedAt
		entries[d.Name()] = entry
	})
	return entry
}

func (c *Cache) Cooldown(name string, kind driver.ErrorKind, now time.Time) {
	duration := map[driver.ErrorKind]time.Duration{
		driver.ErrQuota:       30 * time.Minute,
		driver.ErrAuth:        10 * time.Minute,
		driver.ErrUnavailable: 5 * time.Minute,
	}[kind]
	if duration == 0 {
		return
	}
	_ = c.locked(true, func(entries map[string]Entry) {
		entry := entries[name]
		entry.Health.Name = name
		entry.CooldownUntil, entry.CooldownReason = now.UTC().Add(duration), string(kind)
		entries[name] = entry
	})
}

func (c *Cache) Reset() error {
	return c.locked(true, func(entries map[string]Entry) {
		for name, entry := range entries {
			entry.CooldownUntil, entry.CooldownReason = time.Time{}, ""
			entries[name] = entry
		}
	})
}
