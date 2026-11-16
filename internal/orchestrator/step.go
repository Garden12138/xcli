package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/artifact"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/proc"
	"github.com/Garden12138/xcli/internal/recipe"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/Garden12138/xcli/internal/workspace"
)

func fail(out *StepOutcome, kind driver.ErrorKind, message string) {
	out.Result.Status = api.StatusFailed
	if kind == driver.ErrCancelled {
		out.Result.Status = api.StatusCancelled
	}
	out.Result.Error = &api.Error{Kind: string(kind), Message: message, StepID: out.Result.ID}
}

func accept(out *StepOutcome, result driver.Result) bool {
	out.Text = result.Text
	if result.SessionID != "" {
		out.Result.SessionID = result.SessionID
	}
	out.Result.Usage = result.Usage
	if result.ErrorKind != driver.ErrNone {
		fail(out, result.ErrorKind, result.Error)
		return false
	}
	return true
}

func outside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func (e *Env) invocation(in recipe.Input, agent, id string, round int, schema []byte) (driver.Invocation, error) {
	d := e.Drivers[agent]
	if len(schema) > 0 && d != nil && !d.NativeSchema() {
		in.InlineSchema = schema
	}
	prompt, err := recipe.Render(in)
	if err != nil {
		return driver.Invocation{}, err
	}
	permission := api.PermRead
	if in.Kind == api.KindCode || in.Kind == api.KindImage {
		permission = e.Req.Permission
		if permission == "" {
			permission = e.Config.Defaults.Permission
		}
	}
	inv := driver.Invocation{Prompt: prompt, Workdir: e.WS.Dir, Permission: permission, Images: e.WS.Images}
	if len(e.WS.Inputs)+len(e.WS.Images) > 0 && outside(e.WS.Dir, e.WS.InputsDir) {
		inv.AddDirs = append(inv.AddDirs, e.WS.InputsDir)
	}
	for _, upstream := range in.Upstream {
		for _, path := range upstream.Artifacts {
			if dir := filepath.Dir(path); outside(e.WS.Dir, dir) {
				inv.AddDirs = append(inv.AddDirs, dir)
			}
		}
	}
	for _, path := range []string{in.OutputDir, filepath.Dir(in.DiffFile)} {
		if path != "" && path != "." && outside(e.WS.Dir, path) {
			inv.AddDirs = append(inv.AddDirs, path)
		}
	}
	if len(schema) > 0 && d != nil && d.NativeSchema() {
		dir, err := e.logDir(id, round)
		if err != nil {
			return inv, err
		}
		inv.SchemaFile = filepath.Join(dir, "schema.json")
		err = os.WriteFile(inv.SchemaFile, schema, 0600)
		if err != nil {
			return inv, err
		}
	}
	return inv, nil
}

func (e *Env) jsonRun(ctx context.Context, s api.Step, agent string, in recipe.Input, schema []byte, round int, session string) (driver.Result, json.RawMessage, int) {
	var result driver.Result
	var validation error
	for attempt := 0; attempt < 2; attempt++ {
		runner := *e
		current := round + attempt
		if e.logSubdir != "" && attempt > 0 {
			runner.logSubdir = filepath.Join(e.logSubdir, "schema-repair")
			current = round
		}
		inv, err := runner.invocation(in, agent, s.ID, current, schema)
		if err != nil {
			return invokeError(err), nil, round + attempt
		}
		inv.SessionID = session
		if attempt > 0 {
			inv.SchemaFile = ""
			inv.Prompt += "\n你上一次的输出不符合 JSON Schema，请修正：\n" + validation.Error() + "\n请只输出修正后的 JSON，Schema：\n" + string(schema)
		}
		result = runner.invoke(ctx, agent, s.ID, current, inv)
		if result.ErrorKind != driver.ErrNone {
			return result, nil, round + attempt
		}
		raw, err := recipe.ExtractJSON(result.Text)
		if err == nil {
			err = recipe.Validate(schema, raw)
		}
		if err == nil {
			return result, raw, round + attempt
		}
		validation = err
		session = result.SessionID
	}
	result.ErrorKind = driver.ErrSchemaMismatch
	result.Error = "输出仍需按 Schema 修正，请检查 Schema 与任务说明后重试"
	return result, nil, round + 1
}

func (e *Env) RunStep(ctx context.Context, s api.Step, agent string, up []recipe.Handoff) (out StepOutcome) {
	state := e.state()
	scoped := *e
	e = &scoped
	e.shared = state
	e.kind = s.Kind
	start := e.now()
	out.Result = api.StepResult{ID: s.ID, Kind: s.Kind, Agent: agent, Status: api.StatusRunning, Rounds: 1, Fallbacks: []string{}}
	out.Artifacts = []api.Artifact{}
	defer func() {
		if out.Result.Status == api.StatusRunning {
			out.Result.Status = api.StatusCompleted
		}
		out.Result.Summary = out.Text
		out.Result.DurationMS = e.now().Sub(start).Milliseconds()
		_ = e.Task.Update(func(r *store.Record) {
			for i := range r.Steps {
				if r.Steps[i].ID == s.ID {
					r.Steps[i] = out.Result
					return
				}
			}
			r.Steps = append(r.Steps, out.Result)
		})
		e.emit("step.finished", s.ID, agent, map[string]any{"status": out.Result.Status, "summary": out.Text, "error": out.Result.Error})
	}()
	in := recipe.Input{Kind: s.Kind, Agent: agent, Goal: e.Req.Goal, Instruction: s.Instruction, Workdir: e.WS.Dir, Inputs: e.WS.Inputs, Images: e.WS.Images, Upstream: up, Round: 1}
	if (s.Kind == api.KindCode || s.Kind == api.KindImage) && e.Req.Permission == api.PermRead {
		fail(&out, driver.ErrPermissionDenied, "code/image 需要 write 或 full 权限，请调整 permission 后重试")
		return
	}
	switch s.Kind {
	case api.KindAsk:
		if e.Req.HasSchema() {
			var result driver.Result
			result, out.JSON, out.Result.Rounds = e.jsonRun(ctx, s, agent, in, e.Req.Schema, 1, e.ResumeSession)
			accept(&out, result)
		} else {
			inv, err := e.invocation(in, agent, s.ID, 1, nil)
			if err != nil {
				accept(&out, invokeError(err))
				return
			}
			inv.SessionID = e.ResumeSession
			accept(&out, e.invoke(ctx, agent, s.ID, 1, inv))
		}
	case api.KindImage:
		in.Instruction = "步骤 ID：" + s.ID + "\n" + in.Instruction
		in.OutputDir = e.Req.OutputDir
		if in.OutputDir == "" {
			in.OutputDir = e.Task.Path("out", s.ID)
		} else {
			plan := e.Task.Snapshot().Plan
			if plan != nil && len(plan.Steps) > 1 {
				folder := s.ID
				if s.Output != nil && *s.Output != "" {
					folder = *s.Output
				}
				if filepath.IsAbs(folder) || folder == ".." || strings.HasPrefix(filepath.Clean(folder), ".."+string(os.PathSeparator)) {
					fail(&out, driver.ErrFailed, "请将步骤 output 设为输出目录内的相对路径")
					return
				}
				in.OutputDir = filepath.Join(in.OutputDir, folder)
			}
		}
		if err := os.MkdirAll(in.OutputDir, 0700); err != nil {
			accept(&out, invokeError(err))
			return
		}
		inv, err := e.invocation(in, agent, s.ID, 1, recipe.ImageResultSchema())
		if err != nil {
			accept(&out, invokeError(err))
			return
		}
		inv.SessionID = e.ResumeSession
		if !accept(&out, e.invoke(ctx, agent, s.ID, 1, inv)) {
			return
		}
		var files struct{ Files []string }
		if raw, err := recipe.ExtractJSON(out.Text); err == nil {
			_ = json.Unmarshal(raw, &files)
		}
		out.Artifacts, err = artifact.CollectImages(artifact.CollectOptions{OutDir: in.OutputDir, Since: start, Files: files.Files, Text: out.Text, ExtraDirs: e.Config.Agents[agent].ArtifactDirs, Prefix: s.ID})
		if err != nil {
			accept(&out, invokeError(err))
			return
		}
		if len(out.Artifacts) == 0 {
			fail(&out, driver.ErrNoArtifact, "请检查图片工具和输出目录，或选择其他 CLI 重试")
			return
		}
		for i := range out.Artifacts {
			out.Artifacts[i].StepID, out.Artifacts[i].Agent = s.ID, agent
			e.emit("artifact", s.ID, agent, out.Artifacts[i])
		}
	case api.KindReview:
		dir, err := e.logDir(s.ID, 1)
		if err != nil {
			accept(&out, invokeError(err))
			return
		}
		if e.WS.Git {
			raw, err := exec.Command("git", "-C", e.WS.Dir, "diff", "--no-color", "--binary", "HEAD").Output()
			if err != nil {
				accept(&out, invokeError(err))
				return
			}
			in.DiffFile = filepath.Join(dir, "review.diff")
			if err := os.WriteFile(in.DiffFile, raw, 0600); err != nil {
				accept(&out, invokeError(err))
				return
			}
		}
		result, raw, rounds := e.jsonRun(ctx, s, agent, in, recipe.ReviewSchema(), 1, e.ResumeSession)
		out.Result.Rounds = rounds
		if !accept(&out, result) {
			return
		}
		var review api.ReviewResult
		if err := json.Unmarshal(raw, &review); err != nil {
			accept(&out, invokeError(err))
			return
		}
		review.Agent, review.Round = agent, rounds
		out.Review = &review
		e.emit("review.result", s.ID, agent, map[string]any{"round": rounds, "verdict": review.Verdict, "issues": review.Issues})
	case api.KindCode:
		e.runCode(ctx, s, agent, in, &out)
	default:
		fail(&out, driver.ErrFailed, "请将步骤 kind 设为 ask/code/review/image")
	}
	return
}

func (e *Env) reviewer(s api.Step, coder string) string {
	if s.ReviewBy == nil {
		return ""
	}
	if *s.ReviewBy != "auto" {
		if *s.ReviewBy == coder {
			return ""
		}
		return *s.ReviewBy
	}
	if e.Req.Agent != "" {
		return ""
	}
	selected, score := "", 0
	for _, name := range []string{"codex", "cursor", "antigravity"} {
		d := e.Drivers[name]
		if name == coder || d == nil || e.Health != nil && !e.Health.Available(name, e.now()) {
			continue
		}
		if len(e.Req.Agents) > 0 && !strings.Contains(","+strings.Join(e.Req.Agents, ",")+",", ","+name+",") {
			continue
		}
		if _, err := proc.LookPath(d.Command(), e.PathList); err != nil {
			continue
		}
		if strength := e.Config.Strength(name, "review"); strength > score {
			selected, score = name, strength
		}
	}
	return selected
}

func (e *Env) runCode(ctx context.Context, s api.Step, agent string, in recipe.Input, out *StepOutcome) {
	if err := e.WS.Baseline(); err != nil {
		accept(out, invokeError(err))
		return
	}
	maxFix := e.Config.Defaults.MaxFixRounds
	if e.Req.MaxFixRounds != nil {
		maxFix = *e.Req.MaxFixRounds
	}
	session, previous, feedback, feedbackSummary := e.ResumeSession, "", "", ""
	for round := 1; ; round++ {
		out.Result.Rounds = round
		in.Round, in.Feedback = round, feedback
		inv, err := e.invocation(in, agent, s.ID, round, nil)
		if err != nil {
			accept(out, invokeError(err))
			return
		}
		inv.SessionID = session
		result := e.invoke(ctx, agent, s.ID, round, inv)
		if session != "" && (result.ErrorKind == driver.ErrProtocol || result.ErrorKind == driver.ErrFailed) {
			dir, _ := e.logDir(s.ID, round)
			text := strings.ToLower(tail(filepath.Join(dir, "stderr.log"), 65536))
			if strings.Contains(text, "session") || strings.Contains(text, "not found") {
				preserve(dir, "resume-failed")
				inv.SessionID = ""
				inv.Prompt += "\n原始需求：" + e.Req.Goal + "\n上一轮总结：" + previous
				e.emit("notice", s.ID, agent, map[string]string{"text": "已开启新会话继续修复"})
				result = e.invoke(ctx, agent, s.ID, round, inv)
			}
		}
		if !accept(out, result) {
			return
		}
		session, previous = out.Result.SessionID, out.Text
		feedback, feedbackSummary = "", ""
		dir, err := e.logDir(s.ID, round)
		if err != nil {
			accept(out, invokeError(err))
			return
		}
		commands := make([]string, len(e.Req.Verify))
		for i, command := range e.Req.Verify {
			commands[i] = "PATH='" + strings.ReplaceAll(e.PathList, "'", "'\"'\"'") + "'; export PATH\n" + command
		}
		verified := workspace.RunVerify(ctx, e.WS.Dir, commands, e.Config.Defaults.VerifyTimeout.D(), dir, round)
		for i := range verified {
			verified[i].Command = e.Req.Verify[i]
			e.emit("verify.result", s.ID, agent, map[string]any{"command": verified[i].Command, "round": round, "passed": verified[i].Passed, "exit_code": verified[i].ExitCode})
			if !verified[i].Passed {
				feedback += "\n验证失败：" + verified[i].Command + "\n" + tail(verified[i].LogFile, 16384)
				feedbackSummary += "验证失败：" + verified[i].Command + "，日志：" + verified[i].LogFile + "\n"
			}
		}
		out.Verify = append(out.Verify, verified...)
		out.Changes, err = e.WS.Changes(e.Task.Path("changes.patch"))
		if err != nil {
			accept(out, invokeError(err))
			return
		}
		out.Files = out.Changes.Files
		if ctx.Err() != nil {
			accept(out, invokeError(ctx.Err()))
			return
		}
		if feedback == "" && s.ReviewBy != nil {
			reviewer := e.reviewer(s, agent)
			if reviewer == "" {
				e.emit("notice", s.ID, agent, map[string]string{"text": "没有可用的其他 CLI，跳过交叉审查"})
			} else {
				reviewEnv := *e
				reviewEnv.logSubdir = filepath.Join(e.logSubdir, "review")
				reviewEnv.kind = api.KindReview
				reviewInput := in
				reviewInput.Kind, reviewInput.Agent, reviewInput.CoderSummary = api.KindReview, reviewer, out.Text
				if out.Changes.DiffFile != "" {
					raw, err := os.ReadFile(out.Changes.DiffFile)
					if err != nil {
						accept(out, invokeError(err))
						return
					}
					reviewInput.DiffFile = filepath.Join(dir, "review.diff")
					if err := os.WriteFile(reviewInput.DiffFile, raw, 0600); err != nil {
						accept(out, invokeError(err))
						return
					}
				}
				dr, raw, _ := reviewEnv.jsonRun(ctx, s, reviewer, reviewInput, recipe.ReviewSchema(), round, "")
				if dr.ErrorKind != driver.ErrNone {
					fail(out, dr.ErrorKind, dr.Error)
					return
				}
				var review api.ReviewResult
				if err := json.Unmarshal(raw, &review); err != nil {
					accept(out, invokeError(err))
					return
				}
				review.Agent, review.Round = reviewer, round
				out.Review = &review
				e.emit("review.result", s.ID, reviewer, map[string]any{"round": round, "verdict": review.Verdict, "issues": review.Issues})
				for _, issue := range review.Issues {
					if issue.Severity == "major" || issue.Severity == "blocker" {
						feedback += fmt.Sprintf("\n%s：%s", issue.Severity, issue.Description)
					}
				}
				if feedback != "" {
					feedbackSummary = feedback
				}
			}
		}
		if feedback == "" {
			return
		}
		if round > maxFix {
			out.Result.Status = api.StatusNeedsAttention
			out.Text += "\n仍需处理：" + feedbackSummary
			return
		}
	}
}
