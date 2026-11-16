package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/orchestrator"
	"github.com/Garden12138/xcli/internal/recipe"
)

func (e *Engine) Normalize(req api.Request, cwd string) (api.Request, error) {
	if err := req.Validate(); err != nil {
		return req, err
	}
	req.Version = api.Version
	if req.Capability == "" {
		req.Capability = api.CapAuto
	}
	if req.Permission == "" {
		req.Permission = e.Config.Defaults.Permission
	}
	if req.Capability == api.CapAsk || req.Capability == api.CapReview {
		req.Permission = api.PermRead
	}
	if (req.Capability == api.CapCode || req.Capability == api.CapImage) && req.Permission == api.PermRead {
		return req, fmt.Errorf("code/image 需要 write 或 full 权限，请调整 permission")
	}
	if (req.Capability == api.CapCode || req.Capability == api.CapReview) && req.Workdir == "" {
		return req, fmt.Errorf("请通过 workdir 或 -C 指定 %s 的工作目录", req.Capability)
	}
	abs := func(path string) string {
		if filepath.IsAbs(path) {
			return filepath.Clean(path)
		}
		return filepath.Join(cwd, path)
	}
	check := func(path string, directory bool) (string, error) {
		path = abs(path)
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("请检查路径 %s：%w", path, err)
		}
		if directory && !info.IsDir() {
			return "", fmt.Errorf("请将 workdir 设为目录：%s", path)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return "", fmt.Errorf("请提供普通文件或目录：%s", path)
		}
		return path, nil
	}
	var err error
	if req.Workdir != "" {
		if req.Workdir, err = check(req.Workdir, true); err != nil {
			return req, err
		}
	}
	for _, paths := range []*[]string{&req.Inputs, &req.Images} {
		copyPaths := make([]string, len(*paths))
		for i, p := range *paths {
			if copyPaths[i], err = check(p, false); err != nil {
				return req, err
			}
		}
		*paths = copyPaths
	}
	if req.OutputDir != "" {
		req.OutputDir = abs(req.OutputDir)
		if info, err := os.Stat(req.OutputDir); err == nil && !info.IsDir() {
			return req, fmt.Errorf("请将 output_dir 设为目录：%s", req.OutputDir)
		} else if err != nil && !os.IsNotExist(err) {
			return req, err
		}
	}
	checkAgent := func(name string) error {
		if !e.Config.Enabled(name) {
			return fmt.Errorf("请启用有效的 CLI %q，可选 codex/cursor/antigravity", name)
		}
		return nil
	}
	names := append([]string{}, req.Agents...)
	if req.Agent != "" {
		names = append(names, req.Agent)
	}
	if req.Reviewer != "" && req.Reviewer != "auto" {
		names = append(names, req.Reviewer)
	}
	for name := range req.Models {
		names = append(names, name)
	}
	for _, name := range names {
		if err := checkAgent(name); err != nil {
			return req, err
		}
	}
	if req.SchemaFile != "" {
		req.Schema, err = os.ReadFile(abs(req.SchemaFile))
		if err != nil {
			return req, fmt.Errorf("请检查 schema_file：%w", err)
		}
		req.SchemaFile = ""
	}
	if req.HasSchema() {
		if err := recipe.CheckSchema(req.Schema); err != nil {
			return req, fmt.Errorf("请修正 JSON Schema：%w", err)
		}
	} else {
		req.Schema = nil
	}
	if req.TimeoutSec == 0 {
		req.TimeoutSec = max(1, int(e.Config.Defaults.TaskTimeout.D()/time.Second))
	}
	if req.Plan != nil {
		if err := orchestrator.ValidatePlan(*req.Plan, req, e.available(req), e.Config.Defaults.MaxSteps); err != nil {
			return req, err
		}
	}
	if strings.TrimSpace(req.ParentTask) != "" {
		if _, err := e.Store.Load(req.ParentTask); err != nil {
			return req, err
		}
	}
	return req, nil
}

func InvalidResult(err error) api.Result {
	return api.Result{Version: api.Version, Status: api.StatusInvalid, Summary: err.Error(), Artifacts: []api.Artifact{}, Steps: []api.StepResult{}, Error: &api.Error{Kind: "invalid_request", Message: err.Error()}}
}
