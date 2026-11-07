package agentservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Garden12138/xcli/internal/nativeadapter"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentstore"
)

func fixture(t *testing.T) (*Service, agentstore.Client, agentpolicy.Profile) {
	t.Helper()
	st, e := agentstore.Open(filepath.Join(t.TempDir(), "state"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	clients := &agentstore.Clients{Root: st.Root}
	c := agentstore.Client{ID: "worklog", Scopes: agentstore.Scopes, Agents: []string{"fake"}, Policies: []string{"test-only"}, Operations: []string{"text_task"}, AllowRemote: true, DailyDispatches: 6, DailyProcessStarts: 6, DailyExecutionSeconds: 3600}
	_, e = clients.Issue(c, false)
	if e != nil {
		t.Fatal(e)
	}
	cfg := agentpolicy.DefaultConfig()
	cfg.DailyDispatches = 6
	cfg.DailyProcessStarts = 6
	cfg.DailyExecutionSeconds = 3600
	p := agentpolicy.Profile{AgentID: "fake", AdapterID: "fake-v1", PolicyID: "test-only", Revision: "1", CapabilityRevision: "fake-cap-1", Location: "remote", Operations: []string{"text_task"}}.Freeze()
	s, e := New(st, clients, cfg, []agentpolicy.Profile{p})
	if e != nil {
		t.Fatal(e)
	}
	return s, c, s.Profiles[0]
}
func createSession(t *testing.T, s *Service, c agentstore.Client, p agentpolicy.Profile) api.Session {
	t.Helper()
	ses, _, e := s.CreateSession(c, api.SessionCreate{AgentID: p.AgentID, AdapterID: p.AdapterID, PolicyID: p.PolicyID, CapabilityRevision: p.CapabilityRevision, Budget: api.Budget{MaxDispatches: 2, MaxProcessStarts: 2, MaxExecutionSeconds: 300}, Context: api.Context{App: "worklog", Workflow: "draft", OpaqueRef: "synthetic"}}, api.ID("key"))
	if e != nil {
		t.Fatal(e)
	}
	return ses
}
func taskRequest(ses api.Session) api.TaskCreate {
	text := "synthetic task"
	return api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "text_task", Input: []api.Part{{Type: "text", Text: &text}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 120}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}}
}
func TestAtomicIdempotency100ConcurrentAndOwnership(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	req := taskRequest(ses)
	var wg sync.WaitGroup
	ids := make(chan string, 100)
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _, e := s.CreateTask(c, req, "concurrent-key")
			if e != nil {
				t.Error(e)
				return
			}
			ids <- out.ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for v := range ids {
		if id != "" && id != v {
			t.Fatal("duplicate task")
		}
		id = v
	}
	_ = s.Store.View(func(st agentstore.State) error {
		if len(st.Tasks) != 1 || len(st.Keys) != 2 {
			t.Fatalf("%d tasks, %d keys", len(st.Tasks), len(st.Keys))
		}
		return nil
	})
	other := c
	other.ID = "gardenflow"
	if _, e := s.GetTask(other, id); e == nil {
		t.Fatal("cross client read")
	}
	if _, e := s.GetSession(other, ses.ID); e == nil {
		t.Fatal("cross client session")
	}
	req.Operation = "image_edit"
	if _, _, e := s.CreateTask(c, req, "concurrent-key"); e == nil || e.Error() != "idempotency_conflict" {
		t.Fatal(e)
	}
}
func TestZeroBudgetCloudAndCAS(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	r := taskRequest(ses)
	r.Consent.AllowRemote = false
	if _, _, e := s.CreateTask(c, r, "no-cloud"); e == nil || e.Error() != "cloud_consent_required" {
		t.Fatal(e)
	}
	r.Consent.AllowRemote = true
	r.ExpectedSessionRevision++
	if _, _, e := s.CreateTask(c, r, "bad-cas-key"); e == nil || e.Error() != "session_revision_changed" {
		t.Fatal(e)
	}
	r.ExpectedSessionRevision--
	s.Config.DailyDispatches = 0
	if _, _, e := s.CreateTask(c, r, "zero-budget"); e == nil || e.Error() != "budget_exhausted" {
		t.Fatal(e)
	}
	_ = s.Store.View(func(st agentstore.State) error {
		if len(st.Tasks) != 0 {
			t.Fatal("rejection accepted task")
		}
		return nil
	})
}
func TestRecoveryDoesNotDispatchAndSealsUnknown(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	out, _, e := s.CreateTask(c, taskRequest(ses), "restart-key")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Recover(); e != nil {
		t.Fatal(e)
	}
	out, e = s.GetTask(c, out.ID)
	if e != nil || out.Status != "blocked" || out.Error.Code != "service_restarted" {
		t.Fatal(out, e)
	}
	ses = createSession(t, s, c, p)
	out, _, e = s.CreateTask(c, taskRequest(ses), "dispatch-key")
	if e != nil {
		t.Fatal(e)
	}
	_ = s.Store.Update(func(st *agentstore.State) error {
		st.Tasks[out.ID].InvocationID = "committed-before-spawn"
		st.Tasks[out.ID].Task.Budget.DispatchesConsumed = 1
		return nil
	})
	if e = s.Recover(); e != nil {
		t.Fatal(e)
	}
	out, _ = s.GetTask(c, out.ID)
	ses, _ = s.GetSession(c, ses.ID)
	if out.Status != "needs_reconcile" || ses.State != "needs_reconcile" || out.Budget.DispatchesConsumed != 1 {
		t.Fatal(out, ses)
	}
}

type fakeDriver struct{ calls int }

func (f *fakeDriver) StartTurn(ctx context.Context, b nativeadapter.BoundRequest, observe func(nativeadapter.Observation) error) nativeadapter.TurnResult {
	f.calls++
	if e := observe(nativeadapter.Observation{Category: "response"}); e != nil {
		return nativeadapter.TurnResult{Code: "execution_state_unknown"}
	}
	zero := 0
	return nativeadapter.TurnResult{Text: "offline fixture result", NativeID: "native-fixture", SessionObserved: true, ExitCode: &zero, Terminal: "success", Protocol: "complete", LocalStopConfirmed: true}
}
func TestDispatchReservationIsDurableAndNeverRefunded(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	out, _, e := s.CreateTask(c, taskRequest(ses), "durable-key")
	if e != nil {
		t.Fatal(e)
	}
	driver := &fakeDriver{}
	if _, e = s.ExecuteNext(context.Background(), driver); e != nil {
		t.Fatal(e)
	}
	out, e = s.GetTask(c, out.ID)
	if e != nil || out.Status != "succeeded" || driver.calls != 1 {
		t.Fatal(out, e, driver.calls)
	}
	_ = s.Store.View(func(st agentstore.State) error {
		r := st.Tasks[out.ID]
		if r.InvocationID == "" || r.Task.Budget.DispatchesConsumed != 1 || st.Sessions[ses.ID].Session.Budget.ExecutionSecondsReserved != 0 {
			t.Fatal(r)
		}
		return nil
	})
	if _, e = s.ExecuteNext(context.Background(), driver); e != nil || driver.calls != 1 {
		t.Fatal("duplicate dispatch", e)
	}
	s.Recover()
	out, _ = s.GetTask(c, out.ID)
	if out.Status != "succeeded" || out.Budget.ProcessStartsConsumed != 1 {
		t.Fatal(out)
	}
}

func TestFailedDispatchCommitNeverStartsDriver(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	_, _, e := s.CreateTask(c, taskRequest(ses), "fsync-dispatch")
	if e != nil {
		t.Fatal(e)
	}
	s.Store.BeforeSync = func() error { return errors.New("injected") }
	d := &fakeDriver{}
	if _, e = s.ExecuteNext(context.Background(), d); e == nil || d.calls != 0 {
		t.Fatal(e, d.calls)
	}
}
func TestExplicitZeroCostIsUnenforceable(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	r := taskRequest(ses)
	r.Budget.MaxCostUSD = []byte("0")
	if _, _, e := s.CreateTask(c, r, "hard-zero-cost"); e == nil || e.Error() != "budget_unenforceable" {
		t.Fatal(e)
	}
}
func TestRevocationEpochCannotResurrectQueuedWork(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	out, _, e := s.CreateTask(c, taskRequest(ses), "revoked-queued")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Clients.Revoke(c.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Clients.Issue(c, true); e != nil {
		t.Fatal(e)
	}
	d := &fakeDriver{}
	if _, e = s.ExecuteNext(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	out, _ = s.GetTask(c, out.ID)
	if out.Status != "blocked" || d.calls != 0 {
		t.Fatal(out, d.calls)
	}
}
func TestMonotonicQueueDeadlineAndLifetimeBudget(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	out, _, e := s.CreateTask(c, taskRequest(ses), "queue-expiry")
	if e != nil {
		t.Fatal(e)
	}
	s.acceptedClocks[out.ID] = time.Now().Add(-31 * time.Second)
	d := &fakeDriver{}
	if _, e = s.ExecuteNext(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	out, _ = s.GetTask(c, out.ID)
	if out.Status != "timed_out" || out.Error.Code != "queue_timeout" || d.calls != 0 {
		t.Fatal(out, d.calls)
	}
}
func TestBudgetRaceAcrossDifferentSessions(t *testing.T) {
	s, c, p := fixture(t)
	a := createSession(t, s, c, p)
	b := createSession(t, s, c, p)
	s.Config.DailyDispatches = 1
	for i, ses := range []api.Session{a, b} {
		if _, _, e := s.CreateTask(c, taskRequest(ses), fmt.Sprintf("budget-race-%d", i)); e != nil {
			t.Fatal(e)
		}
	}
	d := &fakeDriver{}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.ExecuteNext(context.Background(), d); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	_, _ = s.ExecuteNext(context.Background(), d)
	if d.calls != 1 {
		t.Fatal("budget overshoot", d.calls)
	}
	_ = s.Store.View(func(st agentstore.State) error {
		spent := 0
		for _, r := range st.Tasks {
			spent += r.Task.Budget.DispatchesConsumed
		}
		if spent != 1 {
			t.Fatal(spent)
		}
		return nil
	})
}
func TestExpiredTextIsPhysicallyRemovedFromJournal(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	req := taskRequest(ses)
	marker := "private-retention-marker-unique"
	req.Input[0].Text = &marker
	out, _, e := s.CreateTask(c, req, "retention-key")
	if e != nil {
		t.Fatal(e)
	}
	d := &fakeDriver{}
	if _, e = s.ExecuteNext(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	if e = s.Store.Checkpoint(); e != nil {
		t.Fatal(e)
	}
	if e = s.Maintain(time.Now().Add(8 * 24 * time.Hour)); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(s.Store.Root, "journal"))
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(b, []byte(marker)) {
		t.Fatal("expired prompt remains in journal")
	}
	if checkpoint, e := os.ReadFile(filepath.Join(s.Store.Root, "checkpoint.json")); e == nil && bytes.Contains(checkpoint, []byte(marker)) {
		t.Fatal("expired prompt remains in cache")
	}
	_ = s.Store.View(func(st agentstore.State) error {
		if st.Tasks[out.ID].Task.Budget.DispatchesConsumed != 1 {
			t.Fatal("retention refunded dispatch")
		}
		return nil
	})
}
func TestCloudPermissionAndQuarantineAreVisibleWithoutProbe(t *testing.T) {
	s, c, _ := fixture(t)
	c.AllowRemote = false
	caps, e := s.Capabilities(c)
	if e != nil || caps.Agents[0].Operations["text_task"].Availability != "blocked" || caps.Agents[0].Operations["text_task"].Reason != "cloud_consent_required" {
		t.Fatal(caps, e)
	}
	if e = s.Store.Update(func(st *agentstore.State) error { st.Quarantined = true; return nil }); e != nil {
		t.Fatal(e)
	}
	c.AllowRemote = true
	caps, e = s.Capabilities(c)
	if e != nil || caps.Agents[0].Operations["text_task"].Reason != "execution_state_unknown" {
		t.Fatal(caps, e)
	}
}
func TestSessionBusyQueueFullAndCancelAreZeroDispatch(t *testing.T) {
	s, c, p := fixture(t)
	first := createSession(t, s, c, p)
	second := createSession(t, s, c, p)
	out, _, e := s.CreateTask(c, taskRequest(first), "first-pending")
	if e != nil {
		t.Fatal(e)
	}
	latest, _ := s.GetSession(c, first.ID)
	if _, _, e = s.CreateTask(c, taskRequest(latest), "busy-session"); e == nil || e.Error() != "session_busy" {
		t.Fatal(e)
	}
	s.Config.QueueLimit = 1
	if _, _, e = s.CreateTask(c, taskRequest(second), "full-queue"); e == nil || e.Error() != "queue_full" {
		t.Fatal(e)
	}
	out, _, e = s.Cancel(c, out.ID, "cancel-pending")
	if e != nil || out.Status != "canceled" || out.Budget.DispatchesConsumed != 0 {
		t.Fatal(out, e)
	}
	d := &fakeDriver{}
	if _, e = s.ExecuteNext(context.Background(), d); e != nil || d.calls != 0 {
		t.Fatal(e, d.calls)
	}
}

type fixedResultDriver struct{ result nativeadapter.TurnResult }

func (d fixedResultDriver) StartTurn(context.Context, nativeadapter.BoundRequest, func(nativeadapter.Observation) error) nativeadapter.TurnResult {
	return d.result
}
func TestUnconfirmedStopQuarantinesAndOutputLimitDoesNotTruncateSuccess(t *testing.T) {
	for _, mode := range []string{"unknown-stop", "output-limit"} {
		t.Run(mode, func(t *testing.T) {
			s, c, p := fixture(t)
			ses := createSession(t, s, c, p)
			out, _, e := s.CreateTask(c, taskRequest(ses), "result-boundary")
			if e != nil {
				t.Fatal(e)
			}
			zero := 0
			result := nativeadapter.TurnResult{Text: "partial", NativeID: "fixture-native", SessionObserved: true, Terminal: "success", Protocol: "complete", ExitCode: &zero, LocalStopConfirmed: mode != "unknown-stop"}
			if mode == "output-limit" {
				result.Text = strings.Repeat("x", api.MaxText+1)
			}
			if _, e = s.ExecuteNext(context.Background(), fixedResultDriver{result}); e != nil {
				t.Fatal(e)
			}
			out, _ = s.GetTask(c, out.ID)
			if out.Result.Final || out.Status == "succeeded" {
				t.Fatal(out)
			}
			if mode == "unknown-stop" {
				if out.Status != "needs_reconcile" {
					t.Fatal(out)
				}
				caps, _ := s.Capabilities(c)
				if caps.Agents[0].Operations["text_task"].Availability != "blocked" {
					t.Fatal(caps)
				}
			} else if out.Error.Code != "output_limit" || out.Result.Text != nil {
				t.Fatal(out)
			}
		})
	}
}
func TestProfileAndConsentCannotChangeWithinSession(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	r := taskRequest(ses)
	r.Consent.ContextSHA256 = api.Hash([]byte("another context"))
	if _, _, e := s.CreateTask(c, r, "wrong-context"); e == nil || e.Error() != "consent_scope_changed" {
		t.Fatal(e)
	}
	r = taskRequest(ses)
	s.Profiles[0].Revision = "changed"
	s.Profiles[0] = s.Profiles[0].Freeze()
	if _, _, e := s.CreateTask(c, r, "changed-profile"); e == nil || e.Error() != "profile_changed" {
		t.Fatal(e)
	}
}
func TestDeadlineWinsOverLateQueuedCancel(t *testing.T) {
	s, c, p := fixture(t)
	ses := createSession(t, s, c, p)
	out, _, e := s.CreateTask(c, taskRequest(ses), "late-cancel-task")
	if e != nil {
		t.Fatal(e)
	}
	_ = s.Store.Update(func(st *agentstore.State) error {
		st.Tasks[out.ID].Task.DeadlineAt = time.Now().Add(-time.Second)
		return nil
	})
	out, _, e = s.Cancel(c, out.ID, "late-cancel")
	if e != nil || out.Status != "timed_out" || out.Budget.DispatchesConsumed != 0 {
		t.Fatal(out, e)
	}
}
