package usage

import (
	"errors"
	"sort"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/runstore"
)

type Options struct {
	Days  int
	Agent string
	Now   time.Time
}

type Summary struct {
	Agent        string      `json:"agent,omitempty"`
	Tasks        int         `json:"tasks"`
	TrackedTasks int         `json:"tracked_tasks"`
	CostedTasks  int         `json:"costed_tasks"`
	Usage        agent.Usage `json:"usage"`
}

type Report struct {
	Since   *time.Time `json:"since,omitempty"`
	Agent   string     `json:"agent,omitempty"`
	Totals  Summary    `json:"totals"`
	ByAgent []Summary  `json:"by_agent"`
}

func Build(records []runstore.Record, options Options) (Report, error) {
	if options.Days < 0 {
		return Report{}, errors.New("days must be zero or greater")
	}
	result := Report{Agent: options.Agent, ByAgent: []Summary{}}
	if options.Days > 0 {
		now := options.Now
		if now.IsZero() {
			now = time.Now()
		}
		maximumDays := int64(^uint64(0)>>1) / int64(24*time.Hour)
		if int64(options.Days) > maximumDays {
			return Report{}, errors.New("days is too large")
		}
		since := now.UTC().Add(-time.Duration(options.Days) * 24 * time.Hour)
		result.Since = &since
	}

	byAgent := map[string]*Summary{}
	included := func(startedAt time.Time) bool {
		return result.Since == nil || !startedAt.Before(*result.Since)
	}
	add := func(name string, item *agent.Usage) {
		if name == "" || (options.Agent != "" && options.Agent != name) {
			return
		}
		summary, ok := byAgent[name]
		if !ok {
			summary = &Summary{Agent: name}
			byAgent[name] = summary
		}
		summary.Tasks++
		if item != nil {
			summary.TrackedTasks++
			if item.EstimatedCostUSD != nil {
				summary.CostedTasks++
			}
			summary.Usage.Add(item)
		}
	}

	for _, record := range records {
		switch record.Kind {
		case "run":
			if included(record.StartedAt) {
				add(record.Agent, record.Usage)
			}
		case "workflow":
			for _, step := range record.Steps {
				startedAt := step.StartedAt
				if startedAt.IsZero() {
					startedAt = record.StartedAt
				}
				if step.Attempts > 0 && included(startedAt) {
					add(step.Agent, step.Usage)
				}
			}
		}
	}

	names := make([]string, 0, len(byAgent))
	for name := range byAgent {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		summary := *byAgent[name]
		result.ByAgent = append(result.ByAgent, summary)
		result.Totals.Tasks += summary.Tasks
		result.Totals.TrackedTasks += summary.TrackedTasks
		result.Totals.CostedTasks += summary.CostedTasks
		result.Totals.Usage.Add(&summary.Usage)
	}
	return result, nil
}
