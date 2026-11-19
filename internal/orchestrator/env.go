package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/health"
	"github.com/Garden12138/xcli/internal/proc"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/Garden12138/xcli/internal/workspace"
)

type Env struct {
	Config        config.Config
	Drivers       map[string]driver.Driver
	Health        *health.Cache
	Task          *store.Task
	WS            *workspace.Workspace
	Req           api.Request // 已规范化
	PathList      string
	ShellProxy    map[string]string
	LockDir       string // <home>/locks
	ResumeSession string // resume 时第一个步骤续接的会话
	Now           func() time.Time
	Sink          func(api.Event) // 内部事件回调；与持久化按 seq 串行执行
	shared        *envShared
	logSubdir     string
	kind          api.Kind
}

type StepOutcome struct {
	Result    api.StepResult
	Text      string
	JSON      json.RawMessage
	Artifacts []api.Artifact
	Verify    []api.VerifyResult
	Review    *api.ReviewResult
	Changes   *api.Changes
	Files     []string // 变更文件，用于交接
}

type envShared struct{ events sync.Mutex }

var envInit sync.Mutex

func (e *Env) state() *envShared {
	envInit.Lock()
	defer envInit.Unlock()
	if e.shared == nil {
		e.shared = &envShared{}
	}
	return e.shared
}

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e *Env) emit(typ, step, agent string, data any) {
	state := e.state()
	state.events.Lock()
	defer state.events.Unlock()
	if event, err := e.Task.Emit(typ, step, agent, data); err == nil && e.Sink != nil {
		e.Sink(event)
	}
}

func (e *Env) logDir(id string, round int) (string, error) {
	dir, err := e.Task.StepDir(id, round)
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, e.logSubdir)
	return dir, os.MkdirAll(dir, 0700)
}

func (e *Env) status(id, agent string, round int, status api.Status) {
	_ = e.Task.Update(func(r *store.Record) {
		for i := range r.Steps {
			if r.Steps[i].ID == id {
				r.Steps[i].Status = status
				if r.Steps[i].Kind != api.KindCode || e.kind != api.KindReview {
					r.Steps[i].Agent, r.Steps[i].Rounds = agent, round
				}
				return
			}
		}
		r.Steps = append(r.Steps, api.StepResult{ID: id, Kind: e.kind, Agent: agent, Rounds: round, Status: status, Fallbacks: []string{}})
	})
}

func tail(path string, limit int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() > limit {
		_, _ = f.Seek(-limit, io.SeekEnd)
	}
	raw, _ := io.ReadAll(io.LimitReader(f, limit))
	return string(raw)
}

func preserve(dir, prefix string) {
	for _, name := range []string{"prompt.md", "stdout.jsonl", "stderr.log", "result.json"} {
		_ = os.Rename(filepath.Join(dir, name), filepath.Join(dir, prefix+"-"+name))
	}
}

func invokeError(err error) driver.Result {
	kind := driver.ErrFailed
	if errors.Is(err, context.Canceled) {
		kind = driver.ErrCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		kind = driver.ErrTimeout
	}
	return driver.Result{ErrorKind: kind, Error: "请检查任务状态、工作目录和日志目录后重试：" + err.Error()}
}

func (e *Env) invoke(ctx context.Context, agent string, stepID string, round int, inv driver.Invocation) driver.Result {
	d := e.Drivers[agent]
	if d == nil {
		return driver.Result{ErrorKind: driver.ErrNotInstalled, Error: "请启用或安装 " + agent + " 后重试"}
	}
	cfg := e.Config.Agents[agent]
	if model := e.Req.Models[agent]; model != "" {
		inv.Model = model
	} else if cfg.Model != "" {
		inv.Model = cfg.Model
	}
	inv.ExtraArgs, inv.Network = append([]string{}, cfg.ExtraArgs...), e.Config.Defaults.Network
	dir, err := e.logDir(stepID, round)
	if err != nil {
		return invokeError(err)
	}
	schema, _ := os.ReadFile(inv.SchemaFile)
	if agent == "codex" && len(schema) > 0 && !strictSchema(schema) {
		inv.SchemaFile = ""
		inv.Prompt += "\n请只输出符合以下 JSON Schema 的 JSON：\n" + string(schema)
	}
	once := func() driver.Result {
		release, err := proc.AcquireSlot(ctx, e.LockDir, agent, cfg.MaxConcurrency, func() {
			e.status(stepID, agent, round, api.StatusWaiting)
			e.emit("step.waiting", stepID, agent, map[string]string{"reason": "等待 CLI 并发槽位"})
		})
		if err != nil {
			return invokeError(err)
		}
		defer release()
		exe, err := proc.LookPath(d.Command(), e.PathList)
		if err != nil {
			return driver.Result{ErrorKind: driver.ErrNotInstalled, Error: err.Error()}
		}
		args, err := d.Build(inv)
		if err != nil {
			return invokeError(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte(inv.Prompt), 0600); err != nil {
			return invokeError(err)
		}
		e.status(stepID, agent, round, api.StatusRunning)
		kind := e.kind
		if kind == "" {
			kind = api.KindAsk
		}
		e.emit("step.started", stepID, agent, map[string]any{"kind": kind, "round": round})
		parser := d.NewParser()
		out := proc.Run(ctx, proc.Spec{Path: exe, Args: args, Dir: inv.Workdir, Env: proc.CLIEnv(e.PathList, cfg.Env, e.ShellProxy)}, proc.Options{
			Timeout: e.Config.Defaults.StepTimeout.D(), StdoutFile: filepath.Join(dir, "stdout.jsonl"), StderrFile: filepath.Join(dir, "stderr.log"), RedactProxy: true,
			OnLine: func(line []byte) {
				for _, event := range parser.Feed(line) {
					switch event.Kind {
					case "message":
						text := []rune(event.Text)
						if len(text) > 500 {
							text = text[:500]
						}
						e.emit("step.progress", stepID, agent, map[string]string{"text": string(text)})
					case "tool":
						e.emit("step.progress", stepID, agent, map[string]string{"tool": event.Tool})
					case "notice":
						e.emit("notice", stepID, agent, map[string]string{"text": event.Text})
					}
				}
			},
		})
		result := parser.Finish(out.ExitCode, out.StderrTail)
		if out.StartErr != nil {
			result.ErrorKind = driver.ErrFailed
			result.Error = "请检查 CLI 执行文件、工作目录和日志目录后重试"
		}
		if out.TimedOut {
			result.ErrorKind = driver.ErrTimeout
			result.Error = "步骤已超时，请调整 defaults.step_timeout 后重试"
		}
		if out.Cancelled {
			result.ErrorKind = driver.ErrCancelled
			result.Error = "任务已取消，可重新提交任务继续"
		}
		if strings.Contains(out.StderrTail, "[xcli] stdout 单行超过 8 MB") {
			e.emit("notice", stepID, agent, map[string]string{"text": "stdout 单行超过 8 MB，已跳过；请查看原始日志"})
		}
		if e.Health != nil {
			e.Health.Cooldown(agent, result.ErrorKind, e.now())
		}
		raw, err := json.Marshal(result)
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, "result.json"), raw, 0600)
		}
		if err != nil {
			return invokeError(err)
		}
		return result
	}
	result := once()
	if inv.SchemaFile != "" && result.ErrorKind == driver.ErrFailed && schemaRejected(tail(filepath.Join(dir, "stdout.jsonl"), 65536), tail(filepath.Join(dir, "stderr.log"), 65536)) {
		preserve(dir, "schema-failed")
		inv.SchemaFile = ""
		inv.Prompt += "\n请只输出符合以下 JSON Schema 的 JSON：\n" + string(schema)
		inv.SessionID = result.SessionID
		e.emit("notice", stepID, agent, map[string]string{"text": "已改用提示词约束与本地 Schema 校验"})
		return once()
	}
	return result
}

// 只检查原生错误详情；result 中回显的 Schema 和正常正文也含关键字。
func schemaRejected(stdout, stderr string) bool {
	text := stderr
	for _, line := range strings.Split(stdout, "\n") {
		var event struct {
			Type, Event, Message string
			Error                json.RawMessage
			Result               struct{ Error json.RawMessage }
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Event == "result" {
			text += "\n" + string(event.Result.Error)
		}
		if event.Type == "error" || event.Type == "turn.failed" {
			text += "\n" + event.Message + "\n" + string(event.Error)
		}
	}
	text = strings.ToLower(text)
	return strings.Contains(text, "invalid_json_schema") || strings.Contains(text, "invalid schema") || strings.Contains(text, "additionalproperties") || strings.Contains(text, "schema is not supported")
}

func strictSchema(raw []byte) bool {
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil || root["type"] != "object" {
		return false
	}
	var check func(any) bool
	check = func(value any) bool {
		switch v := value.(type) {
		case map[string]any:
			if v["type"] == "object" || v["properties"] != nil {
				if v["additionalProperties"] != false {
					return false
				}
				required, ok := v["required"].([]any)
				if !ok {
					return false
				}
				keys := map[string]bool{}
				for _, key := range required {
					if s, ok := key.(string); ok {
						keys[s] = true
					}
				}
				properties, _ := v["properties"].(map[string]any)
				for key := range properties {
					if !keys[key] {
						return false
					}
				}
			}
			for _, child := range v {
				if !check(child) {
					return false
				}
			}
		case []any:
			for _, child := range v {
				if !check(child) {
					return false
				}
			}
		}
		return true
	}
	return check(root)
}
