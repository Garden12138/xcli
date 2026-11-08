package agentservice

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

// Run is deliberately serial across the entire installation. HTTP request
// lifetimes never own a worker; disconnecting a reader does not cancel a turn.
func (s *Service) Run(ctx context.Context, driver nativeadapter.Driver) error {
	if e := s.Maintain(time.Now().UTC()); e != nil {
		return e
	}
	maintenanceAt := time.Now()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			s.mu.Lock()
			s.stopping = true
			s.mu.Unlock()
			return nil
		}
		if time.Since(maintenanceAt) >= time.Second {
			due, e := s.Store.MaintenanceDue(time.Now().UTC())
			if e != nil {
				return e
			}
			if due {
				if e = s.Maintain(time.Now().UTC()); e != nil {
					return e
				}
			}
			maintenanceAt = time.Now()
		}
		ran, e := s.ExecuteNext(ctx, driver)
		if e != nil {
			return e
		}
		if ran {
			continue
		}
		select {
		case <-ctx.Done():
			continue
		case <-s.wake:
		case <-ticker.C:
		}
	}
}
func (s *Service) ExecuteNext(parent context.Context, driver nativeadapter.Driver) (bool, error) {
	queued, e := s.Store.HasQueued()
	if e != nil || !queued {
		return false, e
	}
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return false, nil
	}
	grantLease, err := s.Clients.LockGrants()
	if err != nil {
		s.mu.Unlock()
		return false, err
	}
	var bound nativeadapter.BoundRequest
	var tr agentstore.TaskRecord
	var dispatch bool
	var remaining time.Duration
	err = s.Store.Update(func(st *agentstore.State) error {
		if st.Quarantined {
			return nil
		}
		for _, active := range st.Tasks {
			if active.Task.Status == "running" || active.Task.Status == "validating" || active.Task.Status == "cancel_requested" {
				return nil
			}
		}
		queue := []*agentstore.TaskRecord{}
		for _, r := range st.Tasks {
			if r.Task.Status == "queued" {
				queue = append(queue, r)
			}
		}
		sort.Slice(queue, func(i, j int) bool {
			if queue[i].Task.CreatedAt.Equal(queue[j].Task.CreatedAt) {
				return queue[i].Task.ID < queue[j].Task.ID
			}
			return queue[i].Task.CreatedAt.Before(queue[j].Task.CreatedAt)
		})
		for _, r := range queue {
			now := time.Now().UTC()
			elapsed := time.Since(s.acceptedClocks[r.Task.ID])
			if s.acceptedClocks[r.Task.ID].IsZero() {
				elapsed = now.Sub(r.Task.CreatedAt)
			}
			if !now.Before(r.Task.DeadlineAt) || elapsed >= min(30*time.Second, time.Duration(r.Request.Budget.TimeoutSeconds)*time.Second) {
				finish(st, r, "timed_out", "queue_timeout")
				continue
			}
			ses := st.Sessions[r.Task.SessionID]
			if ses == nil {
				return errors.New("missing session")
			}
			c, e := s.Clients.Get(r.Owner)
			if e != nil || c.GrantEpoch != r.GrantEpoch || !c.Has("task:create") {
				finish(st, r, "blocked", "scope_denied")
				continue
			}
			p, e := s.profile(ses.Session.AgentID, ses.Session.AdapterID, ses.Session.PolicyID)
			if e == nil {
				e = grant(c, p, r.Task.Operation)
			}
			if e == nil {
				e = p.Check(r.Task.Operation)
			}
			if e == nil && p.Limited != nil {
				e = p.Limited.CheckAuthorization()
				if e == nil && p.Limited.AuthorizationSHA256() != r.CallAuthorizationSHA256 {
					e = api.Failure(403, "policy_denied")
				}
				if e == nil && st.Counters["limited/"+p.Limited.RegistrationSHA256].Starts != 0 {
					e = api.Failure(429, "budget_exhausted")
				}
			}
			if e == nil && p.Binary != "" {
				e = p.CheckBinary()
			}
			if e == nil && p.SHA256 != ses.Session.ProfileSHA256 {
				e = api.Failure(409, "profile_changed")
			}
			if e == nil && !ses.Session.ExpiresAt.After(now) {
				e = api.Failure(410, "object_expired")
			}
			if e == nil && p.Location != "local" && (!c.AllowRemote || !r.Request.Consent.AllowRemote) {
				e = api.Failure(403, "cloud_consent_required")
			}
			if e != nil {
				finish(st, r, "blocked", safeCode(e))
				continue
			}
			day := now.Format("2006-01-02")
			seconds := r.Request.Budget.TimeoutSeconds
			if !s.canReserve(st, c, &ses.Session, seconds, day) {
				finish(st, r, "blocked", "budget_exhausted")
				continue
			}
			// Durable reservation and invocation identity precede all driver work.
			r.InvocationID = api.ID("inv")
			r.ReservedSeconds = seconds
			r.BudgetDay = day
			r.StartedAt = &now
			r.Task.Status = "running"
			r.Task.Revision++
			r.Task.Execution.State = "running"
			r.Task.Budget.DispatchesConsumed = 1
			r.Task.Budget.ProcessStartsConsumed = 1
			if p.Limited != nil {
				st.Counters["limited/"+p.Limited.RegistrationSHA256] = agentstore.Counter{Dispatches: 1, Starts: 1}
			}
			for _, k := range []string{"global/" + day, c.ID + "/" + day} {
				x := st.Counters[k]
				x.Dispatches++
				x.Starts++
				x.Reserved += seconds
				st.Counters[k] = x
			}
			b := &ses.Session.Budget
			b.DispatchesConsumed++
			b.ProcessStartsConsumed++
			b.ExecutionSecondsReserved += seconds
			ses.Session.ContextSHA256 = api.Digest(struct {
				Previous      string
				ConsentSHA256 string
			}{ses.Session.ContextSHA256, r.ConsentSHA256})
			ses.Session.Revision++
			addEvent(r, "task.started", map[string]any{"task_revision": r.Task.Revision})
			bound = nativeadapter.BoundRequest{TaskID: r.Task.ID, SessionID: ses.Session.ID, NativeID: ses.NativeID, Operation: r.Task.Operation, Profile: p}
			for _, part := range r.Request.Input {
				if part.Text != nil {
					bound.Prompt = *part.Text
				}
			}
			tr = *r
			remaining = min(time.Until(r.Task.DeadlineAt), time.Duration(r.Request.Budget.TimeoutSeconds)*time.Second-elapsed, time.Until(ses.Session.ExpiresAt))
			dispatch = true
			break
		}
		return nil
	})
	_ = grantLease.Close()
	s.mu.Unlock()
	if err != nil || !dispatch {
		return false, err
	}
	executionStart := time.Now()
	ctx, cancel := context.WithTimeout(parent, remaining)
	bound.Deadline, _ = ctx.Deadline()
	defer cancel()
	observedError := make(chan error, 1)
	var observationMu sync.Mutex
	lastActivity := map[string]time.Time{}
	observer := func(o nativeadapter.Observation) error {
		observationMu.Lock()
		defer observationMu.Unlock()
		if o.Kind == "activity" {
			if previous := lastActivity[o.Category]; !previous.IsZero() && time.Since(previous) < time.Second {
				return nil
			}
			lastActivity[o.Category] = time.Now()
		}

		e := s.Store.Update(func(st *agentstore.State) error {
			r := st.Tasks[tr.Task.ID]
			if r == nil || api.Terminal(r.Task.Status) {
				return api.Failure(409, "profile_changed")
			}
			category := o.Category
			if category != "session" && category != "tool" && category != "response" {
				category = "activity"
			}
			addEvent(r, "task.progress", map[string]any{"category": category})
			return nil
		})
		if e != nil {
			cancel()
			select {
			case observedError <- e:
			default:
			}
		}
		return e
	}
	bound.Workspace, bound.OutputRoot, err = s.Store.Workspace(bound.SessionID, bound.TaskID)
	if bound.Profile.Limited != nil {
		bound.Workspace = bound.Profile.Limited.Workspace
		bound.OutputRoot = ""
	}
	if err != nil {
		return true, err
	}
	if err = s.prepareInputs(&bound, tr); err != nil {
		return true, err
	}
	resultCh := make(chan nativeadapter.TurnResult, 1)
	go func() { resultCh <- driver.StartTurn(ctx, bound, observer) }()
	monitor := time.NewTicker(25 * time.Millisecond)
	defer monitor.Stop()
	var result nativeadapter.TurnResult
	stopping := false
	var cleanupTimer *time.Timer
	var cleanup <-chan time.Time
	ctxDone := ctx.Done()
	stop := func() {
		if !stopping {
			ctxDone = nil
			stopping = true
			cancel()
			cleanupTimer = time.NewTimer(5 * time.Second)
			cleanup = cleanupTimer.C
		}
	}
	for {
		select {
		case result = <-resultCh:
			if cleanupTimer != nil {
				cleanupTimer.Stop()
			}
			goto complete
		case <-ctxDone:
			stop()
		case <-cleanup:
			result = nativeadapter.TurnResult{Terminal: "unknown", Protocol: "unknown", Code: "execution_state_unknown"}
			goto complete
		case <-monitor.C:
			if bound.Profile.Limited != nil && (bound.Profile.Limited.Check(bound.Profile.Native) != nil || bound.Profile.Limited.CheckAuthorization() != nil || bound.Profile.Limited.AuthorizationSHA256() != tr.CallAuthorizationSHA256) {
				stop()
			}
			status, e := s.Store.TaskStatus(tr.Task.ID)
			if e != nil || status == "cancel_requested" {
				stop()
			}

			if current, e := s.Clients.Get(tr.Owner); e != nil || current.GrantEpoch != tr.GrantEpoch {
				stop()
			}
		}
	}
complete:
	select {
	case e := <-observedError:
		return true, e
	default:
	}
	err = s.Store.Update(func(st *agentstore.State) error {
		r := st.Tasks[tr.Task.ID]
		if r == nil || api.Terminal(r.Task.Status) {
			return nil
		}
		ses := st.Sessions[r.Task.SessionID]
		now := time.Now().UTC()
		seconds := time.Since(executionStart).Seconds()
		if seconds > float64(r.ReservedSeconds) {
			seconds = float64(r.ReservedSeconds)
		}
		r.Task.Budget.ExecutionSeconds = seconds
		b := &ses.Session.Budget
		b.ExecutionSeconds += seconds
		b.ExecutionSecondsReserved -= r.ReservedSeconds
		for _, k := range []string{"global/" + r.BudgetDay, r.Owner + "/" + r.BudgetDay} {
			x := st.Counters[k]
			x.Seconds += seconds
			x.Reserved -= r.ReservedSeconds
			st.Counters[k] = x
		}
		r.Task.Execution.NativeExitCode = result.ExitCode
		r.Task.Execution.NativeTerminal = result.Terminal
		r.Task.Execution.LocalStopConfirmed = &result.LocalStopConfirmed
		r.Task.Execution.UpstreamCancelConfirmed = result.UpstreamCancelConfirmed
		r.Task.Protocol = api.Protocol{Status: result.Protocol, NativeSessionObserved: result.SessionObserved}
		r.Task.Usage = result.Usage
		if r.Task.Usage.Source == "" {
			r.Task.Usage = api.Usage{Source: "none", Scope: "unknown"}
		}
		if result.Text != "" && len(result.Text) <= api.MaxText {
			r.Task.Result.Text = &result.Text
		}
		if !result.LocalStopConfirmed {
			r.Task.Execution.State = "unknown"
			r.Task.Execution.SideEffects = "unknown"
			st.Quarantined = true
			finish(st, r, "needs_reconcile", "execution_state_unknown")
			return nil
		}
		r.Task.Execution.State = "completed"
		if ctx.Err() == context.DeadlineExceeded || !time.Now().Before(bound.Deadline) || !now.Before(r.Task.DeadlineAt) {
			r.Task.Execution.State = "stopped"
			r.Task.Execution.SideEffects = "unknown"
			finish(st, r, "timed_out", "timeout")
			return nil
		}
		if r.Task.Status == "cancel_requested" || parent.Err() != nil || stopping {
			r.Task.Execution.State = "stopped"
			r.Task.Execution.SideEffects = "unknown"
			finish(st, r, "canceled", "canceled")
			return nil
		}
		code := result.Code
		if code == "" && result.ExitCode == nil {
			code = "start_failed"
		}
		if code == "" && *result.ExitCode != 0 {
			code = "native_process_failed"
		}
		if code == "" && (!result.SessionObserved || result.NativeID == "") {
			code = "protocol_error"
			if ses.NativeID != "" {
				code = "native_session_lost"
			}
		}
		if code == "" && ses.NativeID != "" && ses.NativeID != result.NativeID {
			code = "native_session_lost"
		}
		if code == "" && (result.Terminal != "success" || result.Protocol != "complete") {
			code = "protocol_error"
		}
		if code == "" && len(result.Text) > api.MaxText {
			code = "output_limit"
		}
		if code != "" {
			r.Task.Execution.State = "failed"
			if result.Terminal != "success" || !result.SessionObserved || code == "native_session_lost" {
				ses.Session.State = "needs_reconcile"
				ses.Session.BindingState = "lost"
			}
			status := "failed"
			switch code {
			case "workspace_untrusted", "native_auth_required", "native_quota_exceeded", "extra_payment_required", "approval_required", "native_tool_unavailable", "artifact_access_blocked", "policy_unenforceable":
				status = "blocked"
			}
			if code == "approval_required" {
				addEvent(r, "approval.required", map[string]any{"category": "tool", "can_approve": false})
			}
			finish(st, r, status, code)
			return nil
		}
		ses.NativeID = result.NativeID
		ses.Session.BindingState = "bound"
		r.Task.Status = "validating"
		r.Task.Revision++
		r.Task.Validation.Status = "running"
		if e := s.verifyInputs(bound, *r); e != nil {
			r.Task.Validation.Status = "failed"
			finish(st, r, "failed", "validation_failed")
			return nil
		}
		if r.Task.Operation != "text_task" {
			return s.validateMedia(st, r, bound, result.Candidates)
		}
		if strings.TrimSpace(result.Text) == "" || !utf8.ValidString(result.Text) {
			r.Task.Validation.Status = "failed"
			finish(st, r, "failed", "validation_failed")
			return nil
		}
		if ctx.Err() == context.DeadlineExceeded || !time.Now().Before(bound.Deadline) {
			r.Task.Execution.SideEffects = "unknown"
			finish(st, r, "timed_out", "timeout")
			return nil
		}
		r.Task.Result.Final = true
		r.Task.Validation.Status = "passed"
		addEvent(r, "text.completed", map[string]any{"result_sha256": api.Hash([]byte(result.Text)), "utf8_bytes": len(result.Text), "result_path": api.Prefix + "/tasks/" + r.Task.ID})
		finish(st, r, "succeeded", "")
		return nil
	})
	return true, err
}
func safeCode(e error) string {
	var wire *api.Error
	if errors.As(e, &wire) {
		return wire.Code
	}
	return "store_unavailable"
}
