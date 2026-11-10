// Package launch is the single authority for xcli native-process reservations.
// Task.Calls and vendor usage are separate orchestration/model metrics.
package launch

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/Garden12138/xcli/internal/localstate"
)

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var ErrBudget = errors.New("launch_budget_exhausted")
var ErrConcurrent = errors.New("launch_concurrency_exhausted")
var ErrUnknown = errors.New("launch_external_baseline_unknown")
var ErrDuplicate = errors.New("launch_request_already_recorded")

type Ref struct {
	Root   string `json:"root"`
	Window string `json:"window"`
}
type External struct {
	Agent     string `json:"agent"`
	Count     *int   `json:"count"`
	Source    string `json:"source"`
	Confirmed bool   `json:"user_confirmed"`
	Scope     string `json:"scope"` // included, excluded, unknown; never inferred
}
type Window struct {
	ID            string    `json:"id"`
	Limit         int       `json:"limit"`
	MaxConcurrent int       `json:"max_concurrent"`
	External      External  `json:"external"`
	CreatedAt     time.Time `json:"created_at"`
}
type Metadata struct {
	TaskID     string `json:"task_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Agent      string `json:"agent"`
	Adapter    string `json:"adapter,omitempty"`
	Kind       string `json:"kind"`
	Version    string `json:"cli_version,omitempty"`
	ParentTask string `json:"parent_task,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Attempt    int    `json:"attempt"`
	Retry      bool   `json:"retry"`
	RequestKey string `json:"request_key,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
}
type Record struct {
	Metadata
	ID          string     `json:"launch_id"`
	Window      string     `json:"window"`
	Executable  string     `json:"executable"`
	Fingerprint string     `json:"fingerprint"` // digest only, never argv/environment
	State       string     `json:"state"`
	StartState  string     `json:"start_state"` // not_started, started, unknown
	ReservedAt  time.Time  `json:"reserved_at"`
	StartedAt   *time.Time `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at"`
	PID         *int       `json:"pid"`
	ExitCode    *int       `json:"exit_code"`
	Reason      string     `json:"termination_reason,omitempty"`
	TimedOut    bool       `json:"timed_out"`
	Canceled    bool       `json:"canceled"`
	Reserved    int        `json:"budget_reserved"`
	Consumed    int        `json:"budget_consumed"`
	Released    int        `json:"budget_released"`
	// Vendor-internal requests/tokens/cost are intentionally not inferred.
	ModelRequests *int     `json:"model_requests"`
	Tokens        *int     `json:"tokens"`
	Cost          *float64 `json:"actual_cost"`
}
type Event struct {
	Revision uint64    `json:"revision"`
	At       time.Time `json:"at"`
	LaunchID string    `json:"launch_id,omitempty"`
	Action   string    `json:"action"`
}
type State struct {
	Version       int               `json:"version"`
	Revision      uint64            `json:"revision"`
	LegacyHistory string            `json:"legacy_history"`
	Windows       map[string]Window `json:"windows"`
	Records       map[string]Record `json:"launches"`
	Events        []Event           `json:"events"`
}
type Summary struct {
	Window          Window `json:"window"`
	Status          string `json:"status"`
	Charged         int    `json:"xcli_charged"`
	ConfirmedStarts int    `json:"confirmed_started"`
	UnknownStarts   int    `json:"unknown_starts"`
	Released        int    `json:"released"`
	Active          int    `json:"active_or_unconfirmed_stopped"`
	Remaining       *int   `json:"remaining"`
	LegacyHistory   string `json:"legacy_history"`
}
type Ticket struct {
	Ref   Ref
	ID    string
	lease *os.File
}

func DefaultRoot() (string, error) {
	p, e := localstate.DataPath()
	return filepath.Join(p, "launches"), e
}
func Open(ref Ref) (Ref, error) {
	if ref.Root == "" {
		var e error
		ref.Root, e = DefaultRoot()
		if e != nil {
			return ref, e
		}
	}
	p, e := filepath.Abs(ref.Root)
	if e != nil {
		return ref, e
	}
	ref.Root = p
	if e = localstate.PrivateDirectory(p); e != nil {
		return ref, e
	}
	return ref, nil
}
func fresh() State {
	return State{Version: 1, LegacyHistory: "UNKNOWN", Windows: map[string]Window{}, Records: map[string]Record{}}
}
func load(ref Ref) (State, error) {
	b, e := localstate.ReadPrivate(filepath.Join(ref.Root, "ledger.json"), 128<<20)
	if os.IsNotExist(e) {
		leases, scanErr := filepath.Glob(filepath.Join(ref.Root, "launch-*.lock"))
		if scanErr != nil || len(leases) > 0 {
			return State{}, errors.New("launch_ledger_missing_with_existing_launch_evidence")
		}
		return fresh(), nil
	}
	if e != nil {
		return State{}, e
	}
	var disk struct {
		Checksum string `json:"checksum"`
		State    State  `json:"state"`
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&disk) != nil || decoder.Decode(&extra) != io.EOF {
		return State{}, errors.New("launch_ledger_corrupt")
	}
	raw, _ := json.Marshal(disk.State)
	hash := sha256.Sum256(raw)
	if disk.Checksum != hex.EncodeToString(hash[:]) {
		return State{}, errors.New("launch_ledger_checksum_invalid")
	}
	s := disk.State
	if s.Version != 1 || s.Windows == nil || s.Records == nil || s.LegacyHistory != "UNKNOWN" {
		return State{}, errors.New("launch_ledger_schema_or_history_invalid")
	}
	if uint64(len(s.Events)) != s.Revision {
		return State{}, errors.New("launch_ledger_revision_invalid")
	}
	for i, event := range s.Events {
		if event.Revision != uint64(i+1) {
			return State{}, errors.New("launch_ledger_event_sequence_invalid")
		}
	}
	for id, w := range s.Windows {
		if id != w.ID || !validName.MatchString(id) || w.Limit < 0 || w.MaxConcurrent < 1 || w.MaxConcurrent > 1000 || w.External.Count != nil && *w.External.Count < 0 {
			return State{}, errors.New("launch_ledger_window_invalid")
		}
	}
	for id, r := range s.Records {
		if _, ok := s.Windows[r.Window]; !ok {
			return State{}, errors.New("launch_ledger_orphan_record")
		}
		if !validName.MatchString(id) || r.ID != id || r.Reserved != 1 || r.Consumed < 0 || r.Consumed > 1 || r.Released < 0 || r.Released > 1 || r.Consumed+r.Released != 1 {
			return State{}, errors.New("launch_ledger_record_invalid")
		}
	}
	return s, nil
}
func save(ref Ref, s *State, action, id string) error {
	s.Revision++
	s.Events = append(s.Events, Event{s.Revision, time.Now().UTC(), id, action})
	payload, _ := json.Marshal(s)
	hash := sha256.Sum256(payload)
	b, e := json.Marshal(struct {
		Checksum string `json:"checksum"`
		State    *State `json:"state"`
	}{hex.EncodeToString(hash[:]), s})
	if e != nil {
		return e
	}
	if len(b) > 128<<20 {
		return errors.New("launch_ledger_capacity")
	}
	return localstate.WritePrivate(filepath.Join(ref.Root, "ledger.json"), b)
}
func transaction(ref Ref, fn func(*State) error) error {
	r, e := Open(ref)
	if e != nil {
		return e
	}
	lock, e := localstate.Lock(filepath.Join(r.Root, "ledger.lock"), true)
	if e != nil {
		return e
	}
	defer lock.Close()
	s, e := load(r)
	if e != nil {
		return e
	}
	return fn(&s)
}
func Create(ref Ref, w Window) error {
	r, e := Open(ref)
	if e != nil {
		return e
	}
	if !validName.MatchString(w.ID) || w.Limit < 0 || w.MaxConcurrent < 1 || w.MaxConcurrent > 1000 || w.External.Count != nil && *w.External.Count < 0 {
		return errors.New("invalid_launch_window")
	}
	if w.External.Scope != "included" && w.External.Scope != "excluded" && w.External.Scope != "unknown" {
		return errors.New("external_scope_required")
	}
	if len(w.External.Source) > 1024 || len(w.External.Agent) > 128 {
		return errors.New("external_baseline_too_large")
	}
	return transaction(r, func(s *State) error {
		if _, ok := s.Windows[w.ID]; ok {
			return errors.New("launch_window_already_exists")
		}
		w.CreatedAt = time.Now().UTC()
		s.Windows[w.ID] = w
		return save(r, s, "window_created", "")
	})
}
func externalKnown(w Window) bool {
	return w.External.Confirmed && w.External.Source != "" && w.External.Agent != "" && (w.External.Scope == "excluded" || w.External.Scope == "included" && w.External.Count != nil)
}
func active(r Record) bool {
	return r.State == "reserved" || r.State == "starting" || r.State == "started" || r.State == "unknown"
}
func summary(s State, w Window) Summary {
	o := Summary{Window: w, Status: "READY", LegacyHistory: s.LegacyHistory}
	for _, r := range s.Records {
		if r.Window != w.ID {
			continue
		}
		o.Charged += r.Consumed
		o.Released += r.Released
		if r.StartState == "started" {
			o.ConfirmedStarts++
		}
		if r.StartState == "unknown" {
			o.UnknownStarts++
		}
		if active(r) {
			o.Active++
		}
	}
	if !externalKnown(w) {
		o.Status = "UNKNOWN"
		return o
	}
	n := w.Limit
	subtract := func(used int) {
		if used >= n {
			n = 0
		} else {
			n -= used
		}
	}
	subtract(o.Charged)
	if w.External.Scope == "included" {
		subtract(*w.External.Count)
	}
	o.Remaining = &n
	if n <= 0 {
		o.Status = "EXHAUSTED"
	}
	if o.UnknownStarts > 0 {
		o.Status = "HAS_UNKNOWN_STARTS"
	}
	return o
}

// Recovery never uses a stale PID as evidence of stopping. The per-launch OS
// lease distinguishes a live xcli owner from a crashed one. A durable starting
// intent has crossed the uncertainty boundary and remains charged/slot-held.
func recoverLocked(ref Ref, s *State) error {
	changed := false
	for id, r := range s.Records {
		if r.State != "reserved" && r.State != "starting" && r.State != "started" {
			continue
		}
		l, e := localstate.Lock(filepath.Join(ref.Root, id+".lock"), false)
		if errors.Is(e, localstate.ErrLocked) {
			continue
		}
		if e != nil {
			return e
		}
		l.Close()
		now := time.Now().UTC()
		r.EndedAt = &now
		if r.State == "reserved" {
			r.State = "not_started"
			r.StartState = "not_started"
			r.Consumed = 0
			r.Released = 1
			r.Reason = "owner_lost_before_start_intent"
		} else {
			r.State = "unknown"
			r.EndedAt = nil
			r.Reason = "owner_lost_start_or_stop_unconfirmed"
		}
		s.Records[id] = r
		changed = true
	}
	if changed {
		return save(ref, s, "crash_recovered", "")
	}
	return nil
}
func Snapshot(ref Ref) (State, error) {
	r, e := Open(ref)
	if e != nil {
		return State{}, e
	}
	var out State
	e = transaction(r, func(s *State) error {
		if e := recoverLocked(r, s); e != nil {
			return e
		}
		out = *s
		return nil
	})
	return out, e
}
func Stats(ref Ref) (Summary, error) {
	s, e := Snapshot(ref)
	if e != nil {
		return Summary{}, e
	}
	w, ok := s.Windows[ref.Window]
	if !ok {
		return Summary{}, errors.New("launch_window_not_found")
	}
	return summary(s, w), nil
}

func Reserve(ref Ref, m Metadata, executable, fingerprint string) (*Ticket, error) {
	r, e := Open(ref)
	if e != nil {
		return nil, e
	}
	if !validName.MatchString(r.Window) {
		return nil, errors.New("launch_window_required")
	}
	if len(m.RequestKey) > 256 || m.Agent == "" || len(m.Evidence) > 1024 {
		return nil, errors.New("launch_metadata_invalid")
	}
	var ticket *Ticket
	e = transaction(r, func(s *State) error {
		if e := recoverLocked(r, s); e != nil {
			return e
		}
		w, ok := s.Windows[r.Window]
		if !ok {
			return errors.New("launch_window_not_found")
		}
		if m.RequestKey != "" {
			for _, old := range s.Records {
				if old.Window == r.Window && old.RequestKey == m.RequestKey {
					if old.Fingerprint != fingerprint {
						return errors.New("launch_request_conflict")
					}
					return fmt.Errorf("%w: %s", ErrDuplicate, old.ID)
				}
			}
		}
		for _, old := range s.Records {
			if old.Window != r.Window && ((m.TaskID != "" && old.TaskID == m.TaskID) || (m.SessionID != "" && old.SessionID == m.SessionID && old.Adapter == m.Adapter)) {
				return errors.New("launch_owner_or_session_window_mismatch")
			}
		}
		if m.Version == "" {
			m.Version = "UNKNOWN"
		}
		o := summary(*s, w)
		if o.Remaining == nil {
			return ErrUnknown
		}
		if *o.Remaining <= 0 {
			return ErrBudget
		}
		if o.Active >= w.MaxConcurrent {
			return ErrConcurrent
		}
		b := make([]byte, 16)
		if _, e := rand.Read(b); e != nil {
			return e
		}
		id := "launch-" + hex.EncodeToString(b)
		if _, exists := s.Records[id]; exists {
			return errors.New("launch_id_collision")
		}
		lease, e := localstate.Lock(filepath.Join(r.Root, id+".lock"), false)
		if e != nil {
			return e
		}
		record := Record{Metadata: m, ID: id, Window: r.Window, Executable: executable, Fingerprint: fingerprint, State: "reserved", StartState: "not_started", ReservedAt: time.Now().UTC(), Reserved: 1, Consumed: 1}
		s.Records[id] = record
		if e = save(r, s, "reserved", id); e != nil {
			lease.Close()
			return e
		}
		ticket = &Ticket{r, id, lease}
		return nil
	})
	return ticket, e
}
func (t *Ticket) change(action string, fn func(*Record)) error {
	return transaction(t.Ref, func(s *State) error {
		r, ok := s.Records[t.ID]
		if !ok {
			return errors.New("launch_record_missing")
		}
		switch action {
		case "start_intent":
			if t.lease == nil || r.State != "reserved" {
				return errors.New("invalid_launch_transition")
			}
		case "started":
			if t.lease == nil || r.State != "starting" {
				return errors.New("invalid_launch_transition")
			}
		case "finished":
			if t.lease == nil || r.State != "started" {
				return errors.New("invalid_launch_transition")
			}
		}
		fn(&r)
		s.Records[t.ID] = r
		return save(t.Ref, s, action, t.ID)
	})
}
func (t *Ticket) Starting() error {
	return t.change("start_intent", func(r *Record) { r.State = "starting"; r.StartState = "unknown" })
}
func (t *Ticket) Started(pid int) error {
	return t.change("started", func(r *Record) {
		now := time.Now().UTC()
		r.State = "started"
		r.StartState = "started"
		r.StartedAt = &now
		r.PID = &pid
	})
}
func (t *Ticket) NotStarted(reason string) error {
	defer t.Close()
	return transaction(t.Ref, func(s *State) error {
		r, ok := s.Records[t.ID]
		if !ok || (r.State != "reserved" && r.State != "starting") || r.StartState == "started" {
			return errors.New("cannot_release_started_or_unknown_launch")
		}
		now := time.Now().UTC()
		r.State = "not_started"
		r.StartState = "not_started"
		r.Consumed = 0
		r.Released = 1
		r.EndedAt = &now
		r.Reason = reason
		s.Records[t.ID] = r
		return save(t.Ref, s, "not_started", t.ID)
	})
}

func (t *Ticket) Finish(code int, canceled, timedOut, stopped bool) error {
	defer t.Close()
	return t.change("finished", func(r *Record) {
		now := time.Now().UTC()
		r.EndedAt = &now
		r.ExitCode = &code
		r.Canceled = canceled
		r.TimedOut = timedOut
		r.Reason = "exited"
		r.State = "finished"
		if canceled {
			r.Reason = "canceled"
		}
		if timedOut {
			r.Reason = "timeout"
		}
		if !stopped {
			r.State = "unknown"
			r.EndedAt = nil
			r.Reason = "stop_unconfirmed"
		}
	})
}
func (t *Ticket) Close() {
	if t.lease != nil {
		_ = t.lease.Close()
		t.lease = nil
	}
}
func Annotate(ref Ref, id, version, session, evidence string) error {
	if id == "" {
		return nil
	}
	r, e := Open(ref)
	if e != nil {
		return e
	}
	return transaction(r, func(s *State) error {
		record, ok := s.Records[id]
		if !ok {
			return errors.New("launch_record_missing")
		}
		if session != "" {
			for _, old := range s.Records {
				if old.Window != record.Window && old.Adapter == record.Adapter && old.SessionID == session {
					return errors.New("launch_session_window_mismatch")
				}
			}
		}
		if version != "" {
			record.Version = version
		}
		if session != "" {
			record.SessionID = session
		}
		if evidence != "" {
			record.Evidence = evidence
		}
		s.Records[id] = record
		return save(r, s, "evidence_attached", id)
	})
}

// ConfirmStopped only clears the concurrency hold. It never refunds a possibly
// started process and does not turn an unknown start into a confirmed start.
func ConfirmStopped(ref Ref, id, evidence string, confirmed bool) error {
	if !confirmed || evidence == "" || len(evidence) > 1024 {
		return errors.New("stop_evidence_and_confirmation_required")
	}
	r, e := Open(ref)
	if e != nil {
		return e
	}
	return transaction(r, func(s *State) error {
		if e := recoverLocked(r, s); e != nil {
			return e
		}
		record, ok := s.Records[id]
		if !ok || record.State != "unknown" {
			return errors.New("launch_is_not_unknown")
		}
		record.State = "unknown_stopped"
		record.Evidence = evidence
		s.Records[id] = record
		return save(r, s, "owner_confirmed_stopped", id)
	})
}
func Digest(parts ...string) string {
	b, _ := json.Marshal(parts)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
