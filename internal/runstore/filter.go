package runstore

import (
	"path/filepath"
	"strings"
	"time"
)

type FilterOptions struct {
	Agent  string
	Kind   string
	Status string
	Cwd    string
	Since  *time.Time
}

func Filter(records []Record, options FilterOptions) []Record {
	result := make([]Record, 0, len(records))
	for _, record := range records {
		if options.Agent != "" && !recordUsesAgent(record, options.Agent) {
			continue
		}
		if options.Kind != "" && record.Kind != options.Kind {
			continue
		}
		if options.Status != "" && record.Status != options.Status {
			continue
		}
		if options.Cwd != "" && filepath.Clean(record.Cwd) != filepath.Clean(options.Cwd) {
			continue
		}
		if options.Since != nil && record.StartedAt.Before(*options.Since) {
			continue
		}
		result = append(result, record)
	}
	return result
}

func WithinProject(record Record, project string) bool {
	if record.Repository != "" && sameOrWithin(project, record.Repository) {
		return true
	}
	return sameOrWithin(project, record.Cwd)
}

func sameOrWithin(parent, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func recordUsesAgent(record Record, name string) bool {
	if record.Agent == name {
		return true
	}
	for _, step := range record.Steps {
		if step.Agent == name {
			return true
		}
	}
	return false
}
