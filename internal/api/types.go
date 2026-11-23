package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const Version = 1

type Capability string

const (
	CapAuto   Capability = "auto"
	CapAsk    Capability = "ask"
	CapCode   Capability = "code"
	CapReview Capability = "review"
	CapImage  Capability = "image"
)

type Kind string

const (
	KindAsk    Kind = "ask"
	KindCode   Kind = "code"
	KindReview Kind = "review"
	KindImage  Kind = "image"
)

type Permission string

const (
	PermRead  Permission = "read"
	PermWrite Permission = "write"
	PermFull  Permission = "full"
)

type Status string

const (
	StatusQueued         Status = "queued"
	StatusRunning        Status = "running"
	StatusCompleted      Status = "completed"
	StatusNeedsAttention Status = "needs_attention"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
	StatusPlanned        Status = "planned"
	StatusInvalid        Status = "invalid"
	StatusPending        Status = "pending"
	StatusWaiting        Status = "waiting"
	StatusSkipped        Status = "skipped"
)

func (s Status) Finished() bool {
	switch s {
	case StatusCompleted, StatusNeedsAttention, StatusFailed, StatusCancelled, StatusPlanned, StatusInvalid:
		return true
	}
	return false
}

type Request struct {
	Version      int               `json:"version,omitempty"`
	Capability   Capability        `json:"capability,omitempty"`
	Goal         string            `json:"goal"`
	Workdir      string            `json:"workdir,omitempty"`
	Inputs       []string          `json:"inputs,omitempty"`
	Images       []string          `json:"images,omitempty"`
	OutputDir    string            `json:"output_dir,omitempty"`
	Schema       json.RawMessage   `json:"schema,omitempty"`
	SchemaFile   string            `json:"schema_file,omitempty"`
	Agent        string            `json:"agent,omitempty"`
	Agents       []string          `json:"agents,omitempty"`
	Models       map[string]string `json:"models,omitempty"`
	Permission   Permission        `json:"permission,omitempty"`
	Isolate      bool              `json:"isolate,omitempty"`
	Review       *bool             `json:"review,omitempty"`
	Reviewer     string            `json:"reviewer,omitempty"`
	Verify       []string          `json:"verify,omitempty"`
	MaxFixRounds *int              `json:"max_fix_rounds,omitempty"`
	Fallback     *bool             `json:"fallback,omitempty"`
	TimeoutSec   int               `json:"timeout_sec,omitempty"`
	Plan         *Plan             `json:"plan,omitempty"`
	PlanOnly     bool              `json:"plan_only,omitempty"`
	ParentTask   string            `json:"parent_task,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// HasSchema 把 JSON 里的 "schema": null 视为没有 Schema。
func (r Request) HasSchema() bool {
	s := strings.TrimSpace(string(r.Schema))
	return s != "" && s != "null"
}

// Validate 只做结构校验；路径、agent 名称、默认值由 engine.Normalize 处理。
func (r Request) Validate() error {
	if r.Version != 0 && r.Version != Version {
		return fmt.Errorf("请将 version 设为 %d，当前收到 %d", Version, r.Version)
	}
	if strings.TrimSpace(r.Goal) == "" {
		return errors.New("请填写 goal 描述任务需求")
	}
	switch r.Capability {
	case "", CapAuto, CapAsk, CapCode, CapReview, CapImage:
	default:
		return fmt.Errorf("未知的 capability %q，可选 auto/ask/code/review/image", r.Capability)
	}
	switch r.Permission {
	case "", PermRead, PermWrite, PermFull:
	default:
		return fmt.Errorf("未知的 permission %q，可选 read/write/full", r.Permission)
	}
	if r.HasSchema() && r.SchemaFile != "" {
		return errors.New("请在 schema 与 schema_file 中选择一种提供方式")
	}
	if r.MaxFixRounds != nil && *r.MaxFixRounds < 0 {
		return errors.New("请将 max_fix_rounds 设为 0 或正整数")
	}
	if r.TimeoutSec < 0 {
		return errors.New("请将 timeout_sec 设为 0 或正整数")
	}
	return nil
}

type Plan struct {
	Source    string `json:"source"`
	Reasoning string `json:"reasoning"`
	Steps     []Step `json:"steps"`
	Planner   string `json:"planner,omitempty"`
}

type Step struct {
	ID          string   `json:"id"`
	Kind        Kind     `json:"kind"`
	Agent       string   `json:"agent"`
	Instruction string   `json:"instruction"`
	DependsOn   []string `json:"depends_on"`
	ReviewBy    *string  `json:"review_by"`
	Output      *string  `json:"output"`
}

type Usage struct {
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
	TotalTokens  int64 `json:"total_tokens,omitempty"`
}

type Artifact struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	MIME   string `json:"mime"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	StepID string `json:"step_id,omitempty"`
	Agent  string `json:"agent,omitempty"`
}

type Changes struct {
	Workdir   string   `json:"workdir"`
	Isolation string   `json:"isolation"`
	Branch    string   `json:"branch,omitempty"`
	DiffFile  string   `json:"diff_file,omitempty"`
	Files     []string `json:"files"`
}

type VerifyResult struct {
	Command  string `json:"command"`
	Round    int    `json:"round"`
	ExitCode int    `json:"exit_code"`
	Passed   bool   `json:"passed"`
	LogFile  string `json:"log_file"`
}

type Issue struct {
	Severity    string  `json:"severity"`
	File        *string `json:"file"`
	Description string  `json:"description"`
}

type ReviewResult struct {
	Agent   string  `json:"agent"`
	Round   int     `json:"round"`
	Verdict string  `json:"verdict"`
	Summary string  `json:"summary"`
	Issues  []Issue `json:"issues"`
}

type Error struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	StepID  string `json:"step_id,omitempty"`
}

type StepResult struct {
	ID         string   `json:"id"`
	Kind       Kind     `json:"kind"`
	Agent      string   `json:"agent"`
	Status     Status   `json:"status"`
	Rounds     int      `json:"rounds,omitempty"`
	SessionID  string   `json:"session_id,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Fallbacks  []string `json:"fallbacks"`
	Usage      *Usage   `json:"usage,omitempty"`
	DurationMS int64    `json:"duration_ms"`
	Error      *Error   `json:"error"`
}

type Result struct {
	Version    int               `json:"version"`
	TaskID     string            `json:"task_id"`
	Status     Status            `json:"status"`
	Capability Capability        `json:"capability"`
	Summary    string            `json:"summary"`
	Output     json.RawMessage   `json:"output"`
	Artifacts  []Artifact        `json:"artifacts"`
	Changes    *Changes          `json:"changes,omitempty"`
	Verify     []VerifyResult    `json:"verify,omitempty"`
	Review     *ReviewResult     `json:"review,omitempty"`
	Plan       *Plan             `json:"plan,omitempty"`
	Steps      []StepResult      `json:"steps"`
	Error      *Error            `json:"error"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	DurationMS int64             `json:"duration_ms"`
}

type Event struct {
	V     int             `json:"v"`
	TS    time.Time       `json:"ts"`
	Task  string          `json:"task"`
	Seq   int             `json:"seq"`
	Type  string          `json:"type"`
	Step  string          `json:"step,omitempty"`
	Agent string          `json:"agent,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func NewEvent(task string, seq int, typ, step, agent string, data any) (Event, error) {
	e := Event{V: Version, TS: time.Now().UTC(), Task: task, Seq: seq, Type: typ, Step: step, Agent: agent}
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return Event{}, err
		}
		e.Data = raw
	}
	return e, nil
}
