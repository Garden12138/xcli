package siwc

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/model"
)

// HistoryOwner binds explicit history to a local app AND OAuth registration.
// Client and registration come from trusted authentication, never request fields.
type HistoryOwner struct{ Client, Registration, Conversation string }
type historyEntry struct {
	input   []model.Object
	expires time.Time
}
type History struct {
	mu      sync.Mutex
	entries map[HistoryOwner]historyEntry
}

func NewHistory() *History { return &History{entries: map[HistoryOwner]historyEntry{}} }
func copyInput(input []model.Object) []model.Object {
	b, _ := json.Marshal(input)
	var out []model.Object
	_ = json.Unmarshal(b, &out)
	return out
}
func (h *History) purge(now time.Time) {
	for k, v := range h.entries {
		if !now.Before(v.expires) {
			delete(h.entries, k)
		}
	}
}

// Commit retains a full bounded message history only after validated completion.
// Failure/cancellation must not advance history. It is memory-only, with TTL and
// a hard capacity; there is no silent truncation or previous_response_id.
func (h *History) Commit(owner HistoryOwner, input []model.Object, terminal Event, now time.Time) error {
	if owner.Client == "" || owner.Registration == "" || owner.Conversation == "" || terminal.Type != "response.completed" {
		return fmt.Errorf("siwc_history_rejected")
	}
	full := copyInput(input)
	full = append(full, model.Object{"role": mustJSON("assistant"), "content": mustJSON(terminal.Text)})
	req := model.Object{"model": mustJSON("contract"), "input": mustJSON(full), "store": mustJSON(false), "stream": mustJSON(true)}
	if err := ValidateRequest(req); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purge(now)
	if _, ok := h.entries[owner]; !ok && len(h.entries) >= 32 {
		return fmt.Errorf("siwc_history_capacity")
	}
	h.entries[owner] = historyEntry{full, now.Add(30 * time.Minute)}
	return nil
}
func (h *History) Load(owner HistoryOwner, now time.Time) []model.Object {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purge(now)
	return copyInput(h.entries[owner].input)
}
func (h *History) ClearRegistration(registration string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k := range h.entries {
		if k.Registration == registration {
			delete(h.entries, k)
		}
	}
}
