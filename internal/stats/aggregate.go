package stats

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/Garden12138/xcli/internal/api"
)

type Filter struct {
	Since, Until time.Time
	App          string
	Location     *time.Location
}

type Report struct {
	Version  int           `json:"version"`
	Range    RangeReport   `json:"range"`
	Overview Overview      `json:"overview"`
	Apps     []AppReport   `json:"apps"`
	Agents   []AgentReport `json:"agents"`
	Errors   []ErrorReport `json:"errors"`
	Daily    []DayReport   `json:"daily"`
	Sources  Sources       `json:"sources"`
}

type RangeReport struct {
	Since    time.Time `json:"since"`
	Until    time.Time `json:"until"`
	Timezone string    `json:"timezone"`
}

type taskCounts struct {
	Tasks          int      `json:"tasks"`
	Succeeded      int      `json:"succeeded"`
	NeedsAttention int      `json:"needs_attention"`
	Failed         int      `json:"failed"`
	Cancelled      int      `json:"cancelled"`
	SuccessRate    *float64 `json:"success_rate"`
}

type tokenCounts struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type Overview struct {
	taskCounts
	tokenCounts
	StepsWithUsage int `json:"steps_with_usage"`
	Steps          int `json:"steps"`
}

type AppReport struct {
	App string `json:"app"`
	taskCounts
	AvgMS       int64     `json:"avg_ms"`
	P50MS       int64     `json:"p50_ms"`
	P90MS       int64     `json:"p90_ms"`
	LastUsed    time.Time `json:"last_used"`
	HasDuration bool      `json:"-"`
	tokenCounts
}

type AgentReport struct {
	Agent        string   `json:"agent"`
	Calls        int      `json:"calls"`
	Succeeded    int      `json:"succeeded"`
	Failed       int      `json:"failed"`
	SuccessRate  *float64 `json:"success_rate"`
	AvgMS        int64    `json:"avg_ms"`
	P90MS        int64    `json:"p90_ms"`
	FallbackFrom int      `json:"fallback_from"`
	FallbackTo   int      `json:"fallback_to"`
	HasDuration  bool     `json:"-"`
	HasUsage     bool     `json:"-"`
	tokenCounts
}

type ErrorReport struct {
	Kind   string   `json:"kind"`
	Count  int      `json:"count"`
	Share  float64  `json:"share"`
	Apps   []string `json:"apps"`
	Agents []string `json:"agents"`
}

type DayReport struct {
	Date        string `json:"date"`
	Tasks       int    `json:"tasks"`
	Succeeded   int    `json:"succeeded"`
	Failed      int    `json:"failed"`
	AvgMS       int64  `json:"avg_ms"`
	HasDuration bool   `json:"-"`
}

type appTotals struct {
	AppReport
	durations []int64
}

type agentTotals struct {
	AgentReport
	durations []int64
}

type errorTotals struct {
	count        int
	apps, agents map[string]bool
}

type dayTotals struct {
	taskCounts
	totalMS int64
}

func Aggregate(records []Record, f Filter) Report {
	if f.Location == nil {
		f.Location = time.Local
	}
	if f.Until.IsZero() {
		f.Until = time.Now()
	}
	report := Report{
		Version: 1, Range: RangeReport{Until: f.Until.In(f.Location), Timezone: f.Location.String()},
		Apps: []AppReport{}, Agents: []AgentReport{}, Errors: []ErrorReport{}, Daily: []DayReport{},
	}
	apps := map[string]*appTotals{}
	agents := map[string]*agentTotals{}
	failures := map[string]*errorTotals{}
	days := map[string]*dayTotals{}
	agentFor := func(name string) *agentTotals {
		if agents[name] == nil {
			agents[name] = &agentTotals{AgentReport: AgentReport{Agent: name}}
		}
		return agents[name]
	}
	first := f.Since
	for _, r := range records {
		at := r.FinishedAt
		if at.IsZero() {
			at = r.StartedAt
		}
		status := api.Status(r.Status)
		if at.IsZero() || at.Before(f.Since) || at.After(f.Until) || !status.Finished() || status == api.StatusInvalid {
			continue
		}
		appName := NormalizeApp(r.App)
		if f.App != "" && appName != f.App {
			continue
		}
		if f.Since.IsZero() && (first.IsZero() || at.Before(first)) {
			first = at
		}
		at = at.In(f.Location)
		report.Overview.addTask(status)
		app := apps[appName]
		if app == nil {
			app = &appTotals{AppReport: AppReport{App: appName}}
			apps[appName] = app
		}
		app.addTask(status)
		if at.After(app.LastUsed) {
			app.LastUsed = at
		}
		date := at.Format(time.DateOnly)
		day := days[date]
		if day == nil {
			day = &dayTotals{}
			days[date] = day
		}
		day.addTask(status)
		if status != api.StatusCancelled {
			app.durations = append(app.durations, r.DurationMS)
			day.totalMS += r.DurationMS
		}
		var errorAgent string
		for _, step := range r.Steps {
			report.Overview.Steps++
			if step.HasUsage {
				report.Overview.StepsWithUsage++
				report.Overview.tokenCounts.add(step.InputTokens, step.OutputTokens)
				app.tokenCounts.add(step.InputTokens, step.OutputTokens)
			}
			for _, fallback := range step.Fallbacks {
				if fallback.Agent != "" {
					agentFor(fallback.Agent).FallbackFrom++
				}
			}
			if stepHasAgent(step) {
				if step.ID == r.ErrorStep {
					errorAgent = step.Agent
				}
				if len(step.Fallbacks) > 0 && step.Status == string(api.StatusCompleted) {
					agentFor(step.Agent).FallbackTo++
				}
			}
		}
		if status == api.StatusFailed {
			row := failures[r.ErrorKind]
			if row == nil {
				row = &errorTotals{apps: map[string]bool{}, agents: map[string]bool{}}
				failures[r.ErrorKind] = row
			}
			row.count++
			row.apps[appName] = true
			if errorAgent != "" {
				row.agents[errorAgent] = true
			}
		}
		for _, call := range r.Calls {
			if call.Agent != "" {
				agentFor(call.Agent).addCall(call, !r.Approximate || hasEstimatedTiming(call, r.Steps))
			}
		}
	}
	report.Overview.setRate()
	for _, app := range apps {
		app.setRate()
		app.AvgMS, app.P50MS, app.P90MS = durationStats(app.durations)
		app.HasDuration = len(app.durations) > 0
		report.Apps = append(report.Apps, app.AppReport)
	}
	for _, agent := range agents {
		agent.SuccessRate = fraction(agent.Succeeded, agent.Calls)
		agent.AvgMS, _, agent.P90MS = durationStats(agent.durations)
		agent.HasDuration = len(agent.durations) > 0
		report.Agents = append(report.Agents, agent.AgentReport)
	}
	for kind, row := range failures {
		report.Errors = append(report.Errors, ErrorReport{Kind: kind, Count: row.count,
			Share: float64(row.count) / float64(report.Overview.Failed),
			Apps:  slices.Sorted(maps.Keys(row.apps)), Agents: append([]string{}, slices.Sorted(maps.Keys(row.agents))...)})
	}
	slices.SortFunc(report.Apps, func(a, b AppReport) int {
		return byCount(a.Tasks, b.Tasks, a.App, b.App)
	})
	slices.SortFunc(report.Agents, func(a, b AgentReport) int {
		return byCount(a.Calls, b.Calls, a.Agent, b.Agent)
	})
	slices.SortFunc(report.Errors, func(a, b ErrorReport) int {
		return byCount(a.Count, b.Count, a.Kind, b.Kind)
	})
	if first.IsZero() {
		first = f.Until
	}
	first = first.In(f.Location)
	if f.Since.IsZero() {
		first = midnight(first)
	}
	report.Range.Since = first
	for d := midnight(first); !d.After(report.Range.Until); d = d.AddDate(0, 0, 1) {
		date := d.Format(time.DateOnly)
		row := DayReport{Date: date}
		if day := days[date]; day != nil {
			row.Tasks, row.Succeeded, row.Failed = day.Tasks, day.Succeeded, day.Failed
			if samples := day.Tasks - day.Cancelled; samples > 0 {
				row.AvgMS, row.HasDuration = day.totalMS/int64(samples), true
			}
		}
		report.Daily = append(report.Daily, row)
	}
	return report
}

func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func byCount(a, b int, aName, bName string) int {
	if a != b {
		return cmp.Compare(b, a)
	}
	return cmp.Compare(aName, bName)
}

func (c *taskCounts) addTask(status api.Status) {
	c.Tasks++
	switch status {
	case api.StatusCompleted, api.StatusPlanned:
		c.Succeeded++
	case api.StatusNeedsAttention:
		c.NeedsAttention++
	case api.StatusFailed:
		c.Failed++
	case api.StatusCancelled:
		c.Cancelled++
	}
}

func (c *taskCounts) setRate() {
	c.SuccessRate = fraction(c.Succeeded, c.Tasks-c.Cancelled)
}

func (c *tokenCounts) add(input, output int64) {
	c.InputTokens += input
	c.OutputTokens += output
}

func fraction(n, total int) *float64 {
	if total == 0 {
		return nil
	}
	rate := float64(n) / float64(total)
	return &rate
}

func durationStats(values []int64) (avg, p50, p90 int64) {
	n := len(values)
	if n == 0 {
		return 0, 0, 0
	}
	for _, value := range values {
		avg += value
	}
	slices.Sort(values)
	return avg / int64(n), values[(n+1)/2-1], values[(9*n+9)/10-1]
}

func (a *agentTotals) addCall(call Call, hasTiming bool) {
	a.Calls++
	if call.ErrorKind == "" {
		a.Succeeded++
	} else {
		a.Failed++
	}
	if hasTiming {
		a.durations = append(a.durations, call.DurationMS)
	}
	if call.HasUsage {
		a.HasUsage = true
		a.tokenCounts.add(call.InputTokens, call.OutputTokens)
	}
}

func hasEstimatedTiming(call Call, steps []StepRecord) bool {
	if call.Round != 1 || (call.Purpose != PurposeRun && call.Purpose != PurposeReview) {
		return false
	}
	for _, step := range steps {
		if step.ID == call.Step && step.Agent == call.Agent && stepHasTiming(step) {
			return (step.Kind == string(api.KindReview)) == (call.Purpose == PurposeReview)
		}
	}
	return false
}
