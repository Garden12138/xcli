// Package task owns bounded, foreground tasks. Native sessions remain Run
// details; a task is the identity shared across planning, handoffs and checks.
package task

import (
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/worktree"
)

type Options struct {
	Goal     string
	Agent    string
	Project  string
	Inputs   []string
	Output   string
	Verify   [][]string
	Timeout  time.Duration
	MaxCalls int
}

type Task struct {
	Version     int                `json:"version"`
	ID          string             `json:"id"`
	Goal        string             `json:"goal"`
	Kind        string             `json:"kind"`
	Mode        string             `json:"mode"`
	Agent       string             `json:"agent,omitempty"`
	Coordinator string             `json:"coordinator,omitempty"`
	Candidates  []string           `json:"candidates"`
	Inputs      []Artifact         `json:"inputs,omitempty"`
	Output      string             `json:"output"`
	Cwd         string             `json:"cwd"`
	Workspace   *worktree.Metadata `json:"workspace,omitempty"`
	Status      string             `json:"status"`
	Reason      string             `json:"reason,omitempty"`
	ExitCode    int                `json:"exit_code"`
	StartedAt   time.Time          `json:"started_at"`
	EndedAt     time.Time          `json:"ended_at,omitempty"`
	Deadline    time.Time          `json:"deadline"`
	MaxCalls    int                `json:"max_calls"`
	Calls       int                `json:"calls"`
	Repairs     int                `json:"repairs"`
	Plans       []Plan             `json:"plans,omitempty"`
	Attempts    []Attempt          `json:"attempts,omitempty"`
	Handoffs    []Handoff          `json:"handoffs,omitempty"`
	Artifacts   []Artifact         `json:"artifacts,omitempty"`
	Checks      []Check            `json:"checks,omitempty"`
	Reviews     []Review           `json:"reviews,omitempty"`
	Snapshot    string             `json:"snapshot,omitempty"`
	DiffFile    string             `json:"diff_file,omitempty"`
	Usage       *agent.Usage       `json:"usage,omitempty"`
	Sequence    uint64             `json:"sequence"`
}

type Plan struct {
	CollaborationRequired bool          `json:"collaboration_required"`
	Version               int           `json:"version"`
	Reason                string        `json:"reason"`
	Acceptance            []Requirement `json:"acceptance"`
	Actions               []Action      `json:"actions"`
}

type Requirement struct {
	Path     string   `json:"path"`
	Contains []string `json:"contains,omitempty"`
	Sources  []string `json:"sources,omitempty"`
}

type Action struct {
	Type      string   `json:"type"`
	Role      string   `json:"role,omitempty"`
	Agent     string   `json:"agent,omitempty"`
	Objective string   `json:"objective,omitempty"`
	Argv      []string `json:"argv,omitempty"`
	Cwd       string   `json:"cwd,omitempty"`
}

type Artifact struct {
	Mode      uint32   `json:"mode,omitempty"`
	Path      string   `json:"path"`
	SHA256    string   `json:"sha256"`
	Producer  string   `json:"producer,omitempty"`
	Consumers []string `json:"consumers,omitempty"`
}

type Attempt struct {
	RetryNotices int      `json:"retry_notices,omitempty"`
	RunID        string   `json:"run_id"`
	Role         string   `json:"role"`
	Agent        string   `json:"agent,omitempty"`
	Cwd          string   `json:"cwd"`
	Inputs       []string `json:"inputs,omitempty"`
	Snapshot     string   `json:"snapshot,omitempty"`
}

type Handoff struct {
	RunID      string   `json:"run_id,omitempty"`
	Summary    string   `json:"summary"`
	Facts      []string `json:"facts"`
	Decisions  []string `json:"decisions"`
	Sources    []string `json:"sources"`
	Unresolved []string `json:"unresolved"`
}

type Review struct {
	RunID    string   `json:"run_id,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Snapshot string   `json:"snapshot,omitempty"`
	Passed   *bool    `json:"passed"`
	Findings []string `json:"findings"`
	Coverage []string `json:"coverage"`
	Sources  []string `json:"sources"`
}

type Check struct {
	RunID    string   `json:"run_id,omitempty"`
	Kind     string   `json:"kind"`
	Argv     []string `json:"argv,omitempty"`
	Cwd      string   `json:"cwd"`
	Snapshot string   `json:"snapshot"`
	Status   string   `json:"status"`
	ExitCode int      `json:"exit_code"`
	Detail   string   `json:"detail,omitempty"`
	LogFile  string   `json:"log_file,omitempty"`
}

type Event struct {
	Version  int       `json:"version"`
	TaskID   string    `json:"task_id"`
	Sequence uint64    `json:"sequence"`
	Time     time.Time `json:"time"`
	Type     string    `json:"type"`
	RunID    string    `json:"run_id,omitempty"`
	Role     string    `json:"role,omitempty"`
	Attempt  int       `json:"attempt,omitempty"`
	Message  string    `json:"message,omitempty"`
	Result   any       `json:"result,omitempty"`
}
