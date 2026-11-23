package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/stats"
	"github.com/Garden12138/xcli/internal/testutil"
)

func TestFixedAgentCooldown(t *testing.T) {
	for _, tc := range []struct {
		kind    driver.ErrorKind
		reason  string
		minutes int
	}{
		{driver.ErrAuth, "未登录", 8},
		{driver.ErrQuota, "额度用尽", 28},
		{driver.ErrUnavailable, "服务暂不可用", 3},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			e := testEngine(t, testutil.FakeScript{})
			e.Health.Cooldown("antigravity", tc.kind, time.Now().Add(-2*time.Minute-30*time.Second))
			until := e.Health.Entry("antigravity").CooldownUntil
			task := create(t, e, api.Request{Goal: "hello", Capability: api.CapAuto, Agent: "antigravity"})
			result := e.Run(context.Background(), task, nil)
			checkAvailabilityFailure(t, result, tc.kind, "antigravity", "冷却", tc.reason, "xcli agents --reset",
				fmt.Sprintf("约 %d 分钟后（%s）", tc.minutes, until.Local().Format("15:04")))
			rows, skipped, err := stats.ReadLedger(filepath.Join(e.Home, "stats", "usage.jsonl"))
			if err != nil || skipped != 0 || len(rows) != 1 || rows[0].TaskID != task.ID() ||
				rows[0].ErrorKind != string(tc.kind) || len(rows[0].Calls) != 0 || rows[0].Approximate {
				t.Fatalf("ledger = %+v, skipped = %d, err = %v", rows, skipped, err)
			}
		})
	}
}

func TestFixedAgentNotInstalled(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{})
	e.PathList = filepath.SplitList(e.PathList)[0]
	if err := os.Remove(filepath.Join(e.PathList, "agy")); err != nil {
		t.Fatal(err)
	}
	task := create(t, e, api.Request{Goal: "hello", Capability: api.CapAsk, Agent: "antigravity"})
	checkAvailabilityFailure(t, e.Run(context.Background(), task, nil),
		driver.ErrNotInstalled, "未找到 antigravity（agy），请安装或在配置中修正 command")
}

func TestAutoUsesEarliestCooldown(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{})
	now := time.Now()
	e.Health.Cooldown("codex", driver.ErrAuth, now)
	e.Health.Cooldown("cursor", driver.ErrQuota, now.Add(-29*time.Minute))
	e.Health.Cooldown("antigravity", driver.ErrUnavailable, now)
	task := create(t, e, api.Request{Goal: "hello", Capability: api.CapAuto})
	checkAvailabilityFailure(t, e.Run(context.Background(), task, nil), driver.ErrQuota,
		"codex 正在冷却（未登录）", "cursor 正在冷却（额度用尽）", "antigravity 正在冷却（服务暂不可用）")
}

func TestExplicitPlanChecksAgentsBeforeAnyStep(t *testing.T) {
	reviewer := "cursor"
	for _, tc := range []struct {
		name, cooled string
		step         api.Step
	}{
		{"step agent", "antigravity", api.Step{ID: "target", Agent: "antigravity", Kind: api.KindAsk}},
		{"review_by", "cursor", api.Step{ID: "target", Agent: "codex", Kind: api.KindCode, ReviewBy: &reviewer}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testEngine(t, testutil.FakeScript{})
			e.Health.Cooldown(tc.cooled, driver.ErrQuota, time.Now())
			plan := &api.Plan{Steps: []api.Step{{ID: "first", Agent: "codex", Kind: api.KindAsk}, tc.step}}
			task := create(t, e, api.Request{Goal: "hello", Plan: plan})
			result := e.Run(context.Background(), task, nil)
			checkAvailabilityFailure(t, result, driver.ErrQuota, tc.cooled, "冷却")
			if result.Error.StepID != tc.step.ID {
				t.Fatalf("error step ID = %q, want %q", result.Error.StepID, tc.step.ID)
			}
		})
	}
}

func TestDuplicatePlanStepIsInvalidRequest(t *testing.T) {
	e := testEngine(t, testutil.FakeScript{})
	step := api.Step{ID: "duplicate", Agent: "antigravity", Kind: api.KindAsk}
	plan := &api.Plan{Steps: []api.Step{step, step}}
	req := api.Request{Goal: "hello", Plan: plan}
	e.Health.Cooldown("antigravity", driver.ErrAuth, time.Now())
	_, err := e.Create(req)
	if err == nil || InvalidResult(err).Error.Kind != "invalid_request" || !strings.Contains(err.Error(), "重复步骤") {
		t.Fatalf("Create: %v", err)
	}
	req.TimeoutSec = 30
	task, err := e.Store.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	checkAvailabilityFailure(t, e.Run(context.Background(), task, nil), "invalid_request", "重复步骤")
}

func checkAvailabilityFailure(t *testing.T, result api.Result, kind driver.ErrorKind, fragments ...string) {
	t.Helper()
	if result.Status != api.StatusFailed || result.Error == nil || result.Error.Kind != string(kind) {
		t.Fatalf("result = %+v, error = %+v, want failed %q", result, result.Error, kind)
	}
	for _, fragment := range fragments {
		if !strings.Contains(result.Error.Message, fragment) {
			t.Errorf("message %q does not contain %q", result.Error.Message, fragment)
		}
	}
	if calls := testutil.FakeCalls(t); len(calls) != 0 {
		t.Fatalf("started fake CLIs: %+v", calls)
	}
}
