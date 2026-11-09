// Package agentservice implements single-turn tasks, separate from internal/task.
package agentservice

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentstore"
)

type Service struct {
	Store          *agentstore.Store
	Clients        *agentstore.Clients
	Config         agentpolicy.Config
	Profiles       []agentpolicy.Profile
	mu             sync.Mutex
	stopping       bool
	wake           chan struct{}
	acceptedClocks map[string]time.Time
}

func New(store *agentstore.Store, clients *agentstore.Clients, cfg agentpolicy.Config, profiles []agentpolicy.Profile) (*Service, error) {
	if e := cfg.Validate(); e != nil {
		return nil, e
	}
	s := &Service{Store: store, Clients: clients, Config: cfg, wake: make(chan struct{}, 1), acceptedClocks: map[string]time.Time{}}
	for _, p := range profiles {
		if p.AdapterID == agentpolicy.AppServerAdapter && !cfg.CodexAppServerEnabled {
			p.Blocker = "policy_unenforceable"
		}
		p.CapabilityRevision = api.Digest(profiles)
		s.Profiles = append(s.Profiles, p.Freeze())
	}
	if e := s.Recover(); e != nil {
		return nil, e
	}
	return s, nil
}
func (s *Service) Notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Service) profile(id, adapter, policy string) (agentpolicy.Profile, error) {
	for _, p := range s.Profiles {
		if p.AgentID == id && p.AdapterID == adapter && p.PolicyID == policy {
			return p, nil
		}
	}
	return agentpolicy.Profile{}, api.Failure(404, "agent_not_found")
}
func authorize(c agentstore.Client, scope string) error {
	if !c.Has(scope) {
		return api.Failure(403, "scope_denied")
	}
	return nil
}
func grant(c agentstore.Client, p agentpolicy.Profile, op string) error {
	if p.Limited != nil && c.ID != p.Limited.ClientID {
		return api.Failure(403, "policy_denied")
	}
	if !slices.Contains(c.Agents, p.AgentID) || !slices.Contains(c.Policies, p.PolicyID) {
		return api.Failure(403, "policy_denied")
	}
	if op != "" && !slices.Contains(c.Operations, op) {
		return api.Failure(403, "scope_denied")
	}
	return nil
}
func (s *Service) Capabilities(c agentstore.Client) (api.Capabilities, error) {
	instance, quarantined, e := s.Store.GatewayState()
	catalog := agentpolicy.Catalog(s.Profiles, instance, c.Agents, c.Policies, c.Operations)
	if !c.AllowRemote {
		for i := range catalog.Agents {
			if catalog.Agents[i].InferenceLocation != "local" {
				for op, value := range catalog.Agents[i].Operations {
					value.Availability = "blocked"
					value.Reason = "cloud_consent_required"
					catalog.Agents[i].Operations[op] = value
				}
			}
		}
	}
	blockers, blockErr := s.Store.BinaryBlockers()
	if blockErr != nil {
		return catalog, blockErr
	}
	for _, p := range s.Profiles {
		if reason, ok := blockers[p.SHA256]; ok {
			for i := range catalog.Agents {
				if catalog.Agents[i].AgentID == p.AgentID && catalog.Agents[i].AdapterID == p.AdapterID {
					for op, value := range catalog.Agents[i].Operations {
						value.Availability = "blocked"
						value.Reason = reason
						catalog.Agents[i].Operations[op] = value
					}
				}
			}
		}
	}
	if quarantined {
		for i := range catalog.Agents {
			for op, value := range catalog.Agents[i].Operations {
				value.Availability = "blocked"
				value.Reason = "execution_state_unknown"
				catalog.Agents[i].Operations[op] = value
			}
		}
	}
	return catalog, e
}
func validateBudget(b api.Budget, session bool) error {
	if len(b.MaxCostUSD) > 0 {
		return api.Failure(422, "budget_unenforceable")
	}
	if session {
		if b.MaxDispatches < 1 || b.MaxDispatches > 6 || b.MaxProcessStarts < 1 || b.MaxProcessStarts > 6 || b.MaxExecutionSeconds < 1 || b.MaxExecutionSeconds > 3600 || b.TimeoutSeconds != 0 {
			return api.Failure(400, "invalid_request")
		}
	} else if b.MaxDispatches != 1 || b.MaxProcessStarts != 1 || b.TimeoutSeconds < 1 || b.TimeoutSeconds > 600 || b.MaxExecutionSeconds != 0 {
		return api.Failure(400, "invalid_request")
	}
	return nil
}
func key(c agentstore.Client, route, k string) string {
	return c.ID + "\x00POST\x00" + route + "\x00" + k
}
func replay(st *agentstore.State, k, digest string) (agentstore.Idempotency, bool, error) {
	r, ok := st.Keys[k]
	if !ok {
		return r, false, nil
	}
	if time.Now().After(r.ExpiresAt) {
		return r, false, api.Failure(410, "idempotent_result_expired")
	}
	if r.Digest != digest {
		return r, false, api.Failure(409, "idempotency_conflict")
	}
	return r, true, nil
}
func remember(st *agentstore.State, k, digest, kind, id string, status int) {
	st.Keys[k] = agentstore.Idempotency{Digest: digest, Kind: kind, ObjectID: id, Status: status, ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour)}
}
func (s *Service) CreateSession(c agentstore.Client, r api.SessionCreate, k string) (out api.Session, replayed bool, err error) {
	if err = authorize(c, "session:create"); err != nil {
		return
	}
	if err = validateBudget(r.Budget, true); err != nil {
		return
	}
	if !slices.Contains([]string{"worklog", "gardenflow", "chat-insight", "contribution-os"}, r.Context.App) || r.Context.Workflow == "" || len(r.Context.Workflow) > 64 || len(r.Context.OpaqueRef) > 128 || strings.ContainsAny(r.Context.OpaqueRef, "/\\\r\n") {
		return out, false, api.Failure(400, "invalid_request")
	}
	p, e := s.profile(r.AgentID, r.AdapterID, r.PolicyID)
	if e != nil {
		return out, false, e
	}
	if e = grant(c, p, ""); e != nil {
		return out, false, e
	}
	if r.CapabilityRevision != p.CapabilityRevision {
		return out, false, api.Failure(409, "capability_changed")
	}
	if e = p.Check("text_task"); e != nil {
		return out, false, e
	}
	if p.Limited != nil && (r.Budget.MaxDispatches != 1 || r.Budget.MaxProcessStarts != 1 || r.Budget.MaxExecutionSeconds != 120) {
		return out, false, api.Failure(403, "policy_denied")
	}
	if r.Budget.MaxDispatches > c.DailyDispatches || r.Budget.MaxProcessStarts > c.DailyProcessStarts || r.Budget.MaxExecutionSeconds > c.DailyExecutionSeconds {
		return out, false, api.Failure(429, "budget_exhausted")
	}
	digest := api.Digest(r)
	err = s.Store.Update(func(st *agentstore.State) error {
		idem, ok, e := replay(st, key(c, "/sessions", k), digest)
		if e != nil {
			return e
		}
		if ok {
			rec := st.Sessions[idem.ObjectID]
			if rec == nil {
				return api.Failure(410, "idempotent_result_expired")
			}
			out = rec.Session
			replayed = true
			return nil
		}
		now := time.Now().UTC()
		out = api.Session{APIVersion: 1, ID: api.ID("ses"), Revision: 1, State: "open", BindingState: "unbound", AgentID: p.AgentID, AdapterID: p.AdapterID, CapabilityRevision: p.CapabilityRevision, PolicyID: p.PolicyID, PolicyRevision: p.Revision, ProfileSHA256: p.SHA256, ContextSHA256: api.Digest(r.Context), Budget: api.BudgetState{MaxDispatches: r.Budget.MaxDispatches, MaxProcessStarts: r.Budget.MaxProcessStarts, MaxExecutionSeconds: r.Budget.MaxExecutionSeconds}, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour), InferenceLocation: p.Location}
		st.Sessions[out.ID] = &agentstore.SessionRecord{Session: out, Owner: c.ID, Context: r.Context}
		remember(st, key(c, "/sessions", k), digest, "session", out.ID, 201)
		return nil
	})
	return
}
func session(st *agentstore.State, c agentstore.Client, id string) (*agentstore.SessionRecord, error) {
	r := st.Sessions[id]
	if r == nil || r.Owner != c.ID {
		return nil, api.Failure(404, "not_found")
	}
	if time.Now().After(r.Session.ExpiresAt) {
		return nil, api.Failure(410, "object_expired")
	}
	return r, nil
}
func task(st *agentstore.State, c agentstore.Client, id string) (*agentstore.TaskRecord, error) {
	r := st.Tasks[id]
	if r == nil || r.Owner != c.ID {
		return nil, api.Failure(404, "not_found")
	}
	if time.Now().After(r.Task.ExpiresAt) {
		return nil, api.Failure(410, "object_expired")
	}
	return r, nil
}
func (s *Service) GetSession(c agentstore.Client, id string) (out api.Session, err error) {
	if err = authorize(c, "session:read"); err != nil {
		return
	}
	err = s.Store.View(func(st agentstore.State) error {
		r, e := session(&st, c, id)
		if e == nil {
			out = r.Session
		}
		return e
	})
	return
}
func (s *Service) GetTask(c agentstore.Client, id string) (out api.Task, err error) {
	if err = authorize(c, "task:read"); err != nil {
		return
	}
	err = s.Store.View(func(st agentstore.State) error {
		r, e := task(&st, c, id)
		if e == nil {
			out = r.Task
		}
		return e
	})
	return
}
func (s *Service) CreateTask(c agentstore.Client, r api.TaskCreate, k string) (out api.Task, replayed bool, err error) {
	if err = authorize(c, "task:create"); err != nil {
		return
	}
	if err = validateBudget(r.Budget, false); err != nil {
		return
	}
	if !slices.Contains([]string{"text_task", "image_generate", "image_edit"}, r.Operation) {
		return out, false, api.Failure(422, "capability_missing")
	}
	if len(r.Input) < 1 || len(r.Input) > 5 {
		return out, false, api.Failure(400, "invalid_request")
	}
	texts := 0
	total := 0
	for _, part := range r.Input {
		switch part.Type {
		case "text":
			if part.Text == nil || part.InputID != nil || strings.TrimSpace(*part.Text) == "" || len(*part.Text) > 256<<10 || !utf8.ValidString(*part.Text) {
				return out, false, api.Failure(400, "invalid_request")
			}
			texts++
			total += len(*part.Text)
		case "file":
			if part.InputID == nil || part.Text != nil {
				return out, false, api.Failure(400, "invalid_request")
			}
		default:
			return out, false, api.Failure(400, "invalid_request")
		}
	}
	if texts != 1 || total > 2<<20 {
		return out, false, api.Failure(400, "invalid_request")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return out, false, api.Failure(503, "service_stopping")
	}
	digest := api.Digest(r)
	err = s.Store.Update(func(st *agentstore.State) error {
		rec, e := session(st, c, r.SessionID)
		if e != nil {
			return e
		}
		ses := &rec.Session
		idem, ok, e := replay(st, key(c, "/tasks", k), digest)
		if e != nil {
			return e
		}
		if ok {
			old := st.Tasks[idem.ObjectID]
			if old == nil || time.Now().After(old.Task.ExpiresAt) {
				return api.Failure(410, "idempotent_result_expired")
			}
			out = old.Task
			replayed = true
			return nil
		}
		if ses.ActiveTaskID != nil {
			return api.Failure(409, "session_busy")
		}
		if ses.State != "open" {
			return api.Failure(409, "profile_changed")
		}
		if ses.Revision != r.ExpectedSessionRevision {
			return api.Failure(409, "session_revision_changed")
		}
		if ses.ContextSHA256 != r.Consent.ContextSHA256 {
			return api.Failure(409, "consent_scope_changed")
		}
		p, e := s.profile(ses.AgentID, ses.AdapterID, ses.PolicyID)
		if e != nil {
			return e
		}
		if e = grant(c, p, r.Operation); e != nil {
			return e
		}
		if p.SHA256 != ses.ProfileSHA256 {
			return api.Failure(409, "profile_changed")
		}
		if p.CapabilityRevision != ses.CapabilityRevision {
			return api.Failure(409, "capability_changed")
		}
		if p.Location != "local" && (!c.AllowRemote || !r.Consent.AllowRemote) {
			return api.Failure(403, "cloud_consent_required")
		}
		if e = p.Check(r.Operation); e != nil {
			return e
		}
		if p.Limited != nil {
			if e = p.Limited.CheckAuthorization(); e != nil {
				return e
			}
			if r.Operation != "text_task" || len(r.Input) != 1 || r.Input[0].Type != "text" || r.Input[0].Text == nil || *r.Input[0].Text != agentpolicy.LimitedPrompt || r.Budget.TimeoutSeconds != 120 || rec.NativeID != "" {
				return api.Failure(403, "policy_denied")
			}
			if st.Counters["limited/"+p.Limited.RegistrationSHA256].Starts != 0 {
				return api.Failure(429, "budget_exhausted")
			}
		}
		inputHashes := map[string]string{}
		imageCount, files := 0, 0
		seen := map[string]bool{}
		for _, part := range r.Input {
			if part.Type != "file" {
				continue
			}
			id := *part.InputID
			in, ok := st.Inputs[id]
			if !ok || in.Owner != c.ID || in.Input.SessionID != ses.ID {
				return api.Failure(404, "not_found")
			}
			if time.Now().After(in.Input.ExpiresAt) {
				return api.Failure(410, "object_expired")
			}
			if seen[id] {
				return api.Failure(400, "invalid_request")
			}
			seen[id] = true
			files++
			inputHashes[id] = in.Input.SHA256
			if strings.HasPrefix(in.Input.MediaType, "image/") {
				imageCount++
			} else {
				total += in.Input.ByteLength
			}
		}
		if total > 2<<20 {
			return api.Failure(413, "input_too_large")
		}
		if (r.Operation == "text_task" && imageCount != 0) || (r.Operation == "image_generate" && files != 0) || (r.Operation == "image_edit" && (imageCount != 1 || files != 1)) {
			return api.Failure(422, "capability_missing")
		}
		queued := 0
		for _, t := range st.Tasks {
			if t.Task.Status == "queued" {
				queued++
			}
		}
		if queued >= s.Config.QueueLimit {
			return api.Failure(429, "queue_full")
		}
		if st.Quarantined {
			return api.Failure(503, "adapter_unavailable")
		}
		if !s.canReserve(st, c, ses, r.Budget.TimeoutSeconds, time.Now().UTC().Format("2006-01-02")) {
			return api.Failure(429, "budget_exhausted")
		}
		now := time.Now().UTC()
		out = api.Task{APIVersion: 1, ID: api.ID("tsk"), SessionID: ses.ID, AgentID: ses.AgentID, AdapterID: ses.AdapterID, ProfileSHA256: ses.ProfileSHA256, CapabilityRevision: ses.CapabilityRevision, Operation: r.Operation, Status: "queued", Revision: 1, Execution: api.Execution{State: "not_started", NativeTerminal: "unknown", SideEffects: "none_observed"}, Protocol: api.Protocol{Status: "not_started"}, Validation: api.Validation{Status: "not_run", Bytes: "not_applicable", Provenance: "not_applicable", Semantics: "not_applicable"}, Result: api.Result{Artifacts: []string{}}, Usage: api.Usage{Source: "none", Scope: "unknown"}, CreatedAt: now, DeadlineAt: now.Add(time.Duration(r.Budget.TimeoutSeconds) * time.Second), ExpiresAt: now.Add(7 * 24 * time.Hour)}
		tr := &agentstore.TaskRecord{InputHashes: inputHashes, ConsentSHA256: api.Digest(struct {
			Request api.TaskCreate
			Hashes  map[string]string
		}{r, inputHashes}), GrantEpoch: c.GrantEpoch, Task: out, Owner: c.ID, Request: r, Events: []api.Event{}}
		if p.Limited != nil {
			tr.CallAuthorizationSHA256 = p.Limited.AuthorizationSHA256()
			if tr.CallAuthorizationSHA256 == "" {
				return api.Failure(403, "policy_denied")
			}
		}
		st.Tasks[out.ID] = tr
		ses.ActiveTaskID = &tr.Task.ID
		ses.Revision++
		addEvent(tr, "task.queued", map[string]any{"task_revision": tr.Task.Revision})
		out = tr.Task
		remember(st, key(c, "/tasks", k), digest, "task", out.ID, 202)
		return nil
	})
	if err == nil {
		if !replayed {
			s.acceptedClocks[out.ID] = time.Now()
		}
		s.Notify()
	}
	return
}
func (s *Service) canReserve(st *agentstore.State, c agentstore.Client, ses *api.Session, seconds int, day string) bool {
	b := ses.Budget
	if b.DispatchesConsumed+1 > b.MaxDispatches || b.ProcessStartsConsumed+1 > b.MaxProcessStarts || b.ExecutionSeconds+float64(b.ExecutionSecondsReserved+seconds) > float64(b.MaxExecutionSeconds) {
		return false
	}
	for _, v := range []struct {
		k       string
		d, p, t int
	}{{"global/" + day, s.Config.DailyDispatches, s.Config.DailyProcessStarts, s.Config.DailyExecutionSeconds}, {c.ID + "/" + day, c.DailyDispatches, c.DailyProcessStarts, c.DailyExecutionSeconds}} {
		x := st.Counters[v.k]
		if x.Dispatches+1 > v.d || x.Starts+1 > v.p || x.Seconds+float64(x.Reserved+seconds) > float64(v.t) {
			return false
		}
	}
	return true
}
func addEvent(r *agentstore.TaskRecord, kind string, payload map[string]any) {
	r.Task.LastEventSeq++
	r.Events = append(r.Events, api.Event{APIVersion: 1, TaskID: r.Task.ID, SessionID: r.Task.SessionID, Seq: r.Task.LastEventSeq, Time: time.Now().UTC(), Type: kind, Payload: payload})
	if len(r.Events) > 10000 {
		r.Events = slices.Clone(r.Events[len(r.Events)-10000:])
	}
}
func finish(st *agentstore.State, r *agentstore.TaskRecord, status, code string) {
	if api.Terminal(r.Task.Status) {
		return
	}
	now := time.Now().UTC()
	if status != "succeeded" {
		for _, id := range r.Task.Result.Artifacts {
			art, ok := st.Artifacts[id]
			if ok {
				art.Artifact.DeliveryState = "quarantined"
				if status == "expired" {
					art.Artifact.DeliveryState = "expired"
				}
				st.Artifacts[id] = art
			}
		}
	}
	r.Task.Status = status
	r.Task.Revision++
	r.Task.EndedAt = &now
	if code != "" {
		e := api.Failure(0, code)
		e.Accepted = true
		e.TaskID = &r.Task.ID
		r.Task.Error = e
	}
	kind := "task." + status
	if status == "succeeded" {
		kind = "task.completed"
	}
	payload := map[string]any{"task_revision": r.Task.Revision, "error_code": code}
	if status == "succeeded" {
		payload = map[string]any{"task_revision": r.Task.Revision, "result_sha256": api.Digest(r.Task.Result)}
	}
	addEvent(r, kind, payload)
	ses := st.Sessions[r.Task.SessionID]
	if ses != nil {
		ses.Session.ActiveTaskID = nil
		ses.Session.Revision++
		if ses.Session.State == "closing" {
			ses.Session.State = "closed"
		}
		if status == "needs_reconcile" || r.Task.Execution.SideEffects == "unknown" {
			ses.Session.State = "needs_reconcile"
		}
		ses.Session.ExpiresAt = minTime(now.Add(24*time.Hour), ses.Session.CreatedAt.Add(7*24*time.Hour))
	}
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func (s *Service) Recover() error {
	return s.Store.Update(func(st *agentstore.State) error {
		for _, r := range st.Tasks {
			if api.Terminal(r.Task.Status) || r.Task.Status == "awaiting_review" {
				continue
			}
			if r.InvocationID == "" {
				finish(st, r, "blocked", "service_restarted")
			} else {
				r.Task.Execution.State = "unknown"
				r.Task.Execution.SideEffects = "unknown"
				finish(st, r, "needs_reconcile", "execution_state_unknown")
				st.Quarantined = true
			}
		}
		return nil
	})
}
func (s *Service) Cancel(c agentstore.Client, id, k string) (out api.Task, replayed bool, err error) {
	if err = authorize(c, "task:cancel"); err != nil {
		return
	}
	err = s.Store.Update(func(st *agentstore.State) error {
		r, e := task(st, c, id)
		if e != nil {
			return e
		}
		route := "/tasks/" + id + "/cancel"
		d := api.Digest(struct{}{})
		_, ok, e := replay(st, key(c, route, k), d)
		if e != nil {
			return e
		}
		if ok {
			replayed = true
		} else {
			cancelTask(st, r)
			remember(st, key(c, route, k), d, "task", id, 202)
		}
		out = r.Task
		return nil
	})
	s.Notify()
	return
}
func cancelTask(st *agentstore.State, r *agentstore.TaskRecord) {
	if api.Terminal(r.Task.Status) {
		return
	}
	now := time.Now().UTC()
	if r.Task.Status == "queued" && (!now.Before(r.Task.DeadlineAt) || now.Sub(r.Task.CreatedAt) >= 30*time.Second) {
		finish(st, r, "timed_out", "queue_timeout")
		return
	}
	if r.Task.Status == "awaiting_review" && r.Task.ReviewDeadlineAt != nil && !now.Before(*r.Task.ReviewDeadlineAt) {
		finish(st, r, "expired", "object_expired")
		return
	}
	if r.Task.Status == "queued" || r.Task.Status == "awaiting_review" {
		finish(st, r, "canceled", "canceled")
		return
	}
	r.Task.Status = "cancel_requested"
	r.Task.Revision++
}
func (s *Service) CloseSession(c agentstore.Client, id, k string) (out api.Session, replayed bool, err error) {
	if err = authorize(c, "session:close"); err != nil {
		return
	}
	err = s.Store.Update(func(st *agentstore.State) error {
		r, e := session(st, c, id)
		if e != nil {
			return e
		}
		route := "/sessions/" + id + "/close"
		d := api.Digest(struct{}{})
		_, ok, e := replay(st, key(c, route, k), d)
		if e != nil {
			return e
		}
		if ok {
			replayed = true
			out = r.Session
			return nil
		}
		if r.Session.State != "needs_reconcile" {
			r.Session.State = "closing"
			r.Session.Revision++
		}
		if r.Session.ActiveTaskID != nil {
			cancelTask(st, st.Tasks[*r.Session.ActiveTaskID])
		}
		if r.Session.ActiveTaskID == nil && r.Session.State != "needs_reconcile" {
			r.Session.State = "closed"
		}
		remember(st, key(c, route, k), d, "session", id, 200)
		out = r.Session
		return nil
	})
	s.Notify()
	return
}
func (s *Service) Events(c agentstore.Client, id string, cursor int64) (out []api.Event, terminal bool, latest int64, err error) {
	if err = authorize(c, "task:read"); err != nil {
		return
	}
	err = s.Store.View(func(st agentstore.State) error {
		r, e := task(&st, c, id)
		if e != nil {
			return e
		}
		latest = r.Task.LastEventSeq
		if cursor < 0 || cursor > latest {
			return api.Failure(400, "invalid_cursor")
		}
		if len(r.Events) > 0 && cursor < r.Events[0].Seq-1 {
			return api.Failure(410, "event_cursor_expired")
		}
		out = []api.Event{}
		for _, event := range r.Events {
			if event.Seq > cursor {
				out = append(out, event)
			}
		}
		terminal = api.Terminal(r.Task.Status)
		return nil
	})
	return
}
func IsUnavailable(err error) bool {
	return errors.Is(err, agentstore.ErrUnavailable) || errors.Is(err, agentstore.ErrCorrupt)
}

func (s *Service) StopAccepting() { s.mu.Lock(); s.stopping = true; s.mu.Unlock(); s.Notify() }
