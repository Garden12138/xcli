package health

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/driver"
)

func TestCooldown(t *testing.T) {
	c := Open(t.TempDir())
	now := time.Now()
	c.Cooldown("codex", driver.ErrQuota, now)
	if c.Available("codex", now.Add(29*time.Minute)) || !c.Available("codex", now.Add(31*time.Minute)) {
		t.Fatal("quota cooldown must be 30 minutes")
	}
	c.Cooldown("cursor", driver.ErrAuth, now)
	if c.Available("cursor", now.Add(9*time.Minute)) || !c.Available("cursor", now.Add(11*time.Minute)) {
		t.Fatal("auth cooldown must be 10 minutes")
	}
	c.Cooldown("antigravity", driver.ErrFailed, now)
	if !c.Available("antigravity", now) {
		t.Fatal("generic failures must not start cooldown")
	}
}

func TestCooldownPersists(t *testing.T) {
	home := t.TempDir()
	first, second := Open(home), Open(home)
	now := time.Now()
	first.Cooldown("codex", driver.ErrQuota, now)
	second.Cooldown("cursor", driver.ErrAuth, now)
	if first.Available("cursor", now) || second.Available("codex", now) {
		t.Fatal("separate caches lost persisted cooldown")
	}
}

func TestReset(t *testing.T) {
	c := Open(t.TempDir())
	now := time.Now()
	c.Cooldown("codex", driver.ErrQuota, now)
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if !c.Available("codex", now) || c.Entry("codex").CooldownReason != "" {
		t.Fatal("reset kept cooldown")
	}
}

type fakeDriver struct{ calls int }

func (d *fakeDriver) Name() string                              { return "codex" }
func (d *fakeDriver) Command() string                           { return "fakecli" }
func (d *fakeDriver) NativeSchema() bool                        { return false }
func (d *fakeDriver) NewParser() driver.Parser                  { return nil }
func (d *fakeDriver) Build(driver.Invocation) ([]string, error) { return nil, nil }
func (d *fakeDriver) Probe(context.Context, string, []string) driver.Health {
	d.calls++
	return driver.Health{Name: "codex", Installed: true, Auth: "ok", Version: "fake 1.0"}
}

func TestProbeCached(t *testing.T) {
	c := Open(t.TempDir())
	d := &fakeDriver{}
	for range 2 {
		if got := c.Probe(context.Background(), d, "/fakecli", nil, time.Minute); got.Health.Version != "fake 1.0" {
			t.Fatalf("probe: %+v", got)
		}
	}
	if d.calls != 1 {
		t.Fatalf("probe calls = %d", d.calls)
	}
	c.Cooldown("codex", driver.ErrQuota, time.Now())
	c.Probe(context.Background(), d, "/fakecli", nil, 0)
	if c.Available("codex", time.Now()) {
		t.Fatal("health refresh erased cooldown")
	}
}

func TestProbeReturnsFreshHealthWhenCacheUnavailable(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "cache"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	e := Open(home).Probe(context.Background(), &fakeDriver{}, "/fakecli", nil, time.Minute)
	if !e.Health.Installed || e.Health.Version != "fake 1.0" || e.CheckedAt.IsZero() {
		t.Fatalf("fresh probe lost on cache failure: %+v", e)
	}
}

func TestConcurrentCachesKeepOtherAgents(t *testing.T) {
	home := t.TempDir()
	var wg sync.WaitGroup
	for range 10 {
		for _, name := range []string{"codex", "cursor", "antigravity"} {
			wg.Go(func() { Open(home).Cooldown(name, driver.ErrQuota, time.Now()) })
		}
	}
	wg.Wait()
	for _, name := range []string{"codex", "cursor", "antigravity"} {
		if Open(home).Available(name, time.Now()) {
			t.Errorf("lost %s cooldown", name)
		}
	}
}
