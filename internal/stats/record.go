package stats

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/store"
)

const (
	PurposePlan   = "plan"
	PurposeRun    = "run"
	PurposeRepair = "repair"
	PurposeReview = "review"
)

type Fallback struct {
	Agent  string `json:"agent"`
	Reason string `json:"reason"`
}

type StepRecord struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Agent        string     `json:"agent"`
	Status       string     `json:"status"`
	DurationMS   int64      `json:"duration_ms"`
	Rounds       int        `json:"rounds"`
	Fallbacks    []Fallback `json:"fallbacks"`
	InputTokens  int64      `json:"input_tokens"`
	OutputTokens int64      `json:"output_tokens"`
	HasUsage     bool       `json:"has_usage"`
}

type ReviewRecord struct {
	Step    string `json:"step"`
	Agent   string `json:"agent"`
	Round   int    `json:"round"`
	Verdict string `json:"verdict"`
}

type Call struct {
	Agent        string `json:"agent"`
	Purpose      string `json:"purpose"`
	Step         string `json:"step"`
	Round        int    `json:"round"`
	DurationMS   int64  `json:"duration_ms"`
	ErrorKind    string `json:"error_kind"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	HasUsage     bool   `json:"has_usage"`
}

type Record struct {
	V           int            `json:"v"`
	TaskID      string         `json:"task_id"`
	App         string         `json:"app"`
	Capability  string         `json:"capability"`
	Status      string         `json:"status"`
	ErrorKind   string         `json:"error_kind"`
	ErrorStep   string         `json:"error_step"`
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  time.Time      `json:"finished_at"`
	DurationMS  int64          `json:"duration_ms"`
	ParentTask  string         `json:"parent_task"`
	Planner     string         `json:"planner"`
	Steps       []StepRecord   `json:"steps"`
	Reviews     []ReviewRecord `json:"reviews"`
	Calls       []Call         `json:"calls"`
	Approximate bool           `json:"approximate"`
}

var appPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,39}$`)

// ParseApp normalizes an app name and reports whether it is valid.
func ParseApp(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	return value, appPattern.MatchString(value)
}

func NormalizeApp(value string) string {
	if app, ok := ParseApp(value); ok {
		return app
	}
	return "unknown"
}

func FromTask(rec store.Record, events []api.Event, calls []Call) (Record, bool) {
	if !rec.Status.Finished() || rec.Status == api.StatusInvalid || rec.Result == nil {
		return Record{}, false
	}
	result := rec.Result
	r := Record{
		V: 1, TaskID: rec.ID, App: NormalizeApp(rec.Request.Metadata["app"]), Capability: string(result.Capability),
		Status: string(result.Status), StartedAt: result.StartedAt, FinishedAt: result.FinishedAt, DurationMS: result.DurationMS,
		ParentTask: rec.Request.ParentTask, Steps: make([]StepRecord, 0, len(result.Steps)),
		Reviews: []ReviewRecord{}, Calls: calls, Approximate: calls == nil,
	}
	if result.Error != nil {
		r.ErrorKind, r.ErrorStep = result.Error.Kind, result.Error.StepID
	}
	plan := result.Plan
	if plan == nil {
		plan = rec.Plan
	}
	if plan != nil {
		r.Planner = plan.Planner
	}
	for _, s := range result.Steps {
		step := StepRecord{ID: s.ID, Kind: string(s.Kind), Agent: s.Agent, Status: string(s.Status),
			DurationMS: s.DurationMS, Rounds: s.Rounds, Fallbacks: []Fallback{}}
		for _, fallback := range s.Fallbacks {
			agent, reason, ok := strings.Cut(fallback, ":")
			if ok {
				step.Fallbacks = append(step.Fallbacks, Fallback{Agent: strings.TrimSpace(agent), Reason: strings.TrimSpace(reason)})
			}
		}
		if s.Usage != nil {
			step.InputTokens, step.OutputTokens, step.HasUsage = s.Usage.InputTokens, s.Usage.OutputTokens, true
		}
		r.Steps = append(r.Steps, step)
	}
	for _, ev := range events {
		if ev.Type != "review.result" {
			continue
		}
		var data struct {
			Round   int    `json:"round"`
			Verdict string `json:"verdict"`
		}
		if json.Unmarshal(ev.Data, &data) == nil {
			r.Reviews = append(r.Reviews, ReviewRecord{Step: ev.Step, Agent: ev.Agent, Round: data.Round, Verdict: data.Verdict})
		}
	}
	if r.Approximate {
		r.Calls = estimateCalls(r, result.Steps)
	}
	return r, true
}

func stepHasAgent(step StepRecord) bool {
	if step.Agent == "" || step.Agent == "auto" {
		return false
	}
	switch api.Status(step.Status) {
	case api.StatusSkipped, api.StatusPending, api.StatusWaiting:
		return false
	case api.StatusCancelled:
		return step.Rounds > 0
	}
	return true
}

func stepHasTiming(step StepRecord) bool {
	return stepHasAgent(step) && len(step.Fallbacks) == 0 && max(step.Rounds, 1) == 1
}

func estimateCalls(r Record, results []api.StepResult) []Call {
	calls := []Call{}
	if r.Planner != "" {
		calls = append(calls, Call{Agent: r.Planner, Purpose: PurposePlan, Step: "plan", Round: 1})
	}
	codeSteps := map[string]bool{}
	for i, step := range r.Steps {
		for _, f := range step.Fallbacks {
			calls = append(calls, Call{Agent: f.Agent, Purpose: PurposeRun, Step: step.ID, Round: 1, ErrorKind: f.Reason})
		}
		if !stepHasAgent(step) {
			continue
		}
		purpose := PurposeRun
		rounds := max(step.Rounds, 1)
		switch api.Kind(step.Kind) {
		case api.KindCode:
			codeSteps[step.ID] = true
		case api.KindReview:
			purpose, rounds = PurposeReview, 1
		}
		for round := 1; round < rounds; round++ {
			calls = append(calls, Call{Agent: step.Agent, Purpose: PurposeRepair, Step: step.ID, Round: round})
		}
		if rounds > 1 {
			purpose = PurposeRepair
		}
		call := Call{Agent: step.Agent, Purpose: purpose, Step: step.ID, Round: rounds,
			InputTokens: step.InputTokens, OutputTokens: step.OutputTokens, HasUsage: step.HasUsage}
		switch api.Status(step.Status) {
		case api.StatusCompleted, api.StatusNeedsAttention:
		case api.StatusCancelled:
			call.ErrorKind = "cancelled"
		default:
			call.ErrorKind = "failed"
			if err := results[i].Error; err != nil && err.Kind != "" {
				call.ErrorKind = err.Kind
			}
		}
		if stepHasTiming(step) {
			call.DurationMS = step.DurationMS
		}
		calls = append(calls, call)
	}
	for _, review := range r.Reviews {
		if codeSteps[review.Step] && review.Agent != "" && review.Verdict != "" {
			calls = append(calls, Call{Agent: review.Agent, Purpose: PurposeReview, Step: review.Step, Round: review.Round})
		}
	}
	return calls
}
