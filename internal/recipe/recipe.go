package recipe

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"unicode/utf8"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Handoff struct {
	StepID    string
	Agent     string
	Kind      api.Kind
	Summary   string // 截断到 8 KB
	Artifacts []string
	Files     []string
}

type Input struct {
	Kind         api.Kind
	Agent        string
	Goal         string
	Instruction  string
	Workdir      string
	OutputDir    string
	Inputs       []string
	Images       []string
	Upstream     []Handoff
	Feedback     string
	Round        int
	InlineSchema []byte // 驱动没有原生 Schema 参数时，把 Schema 附在提示词里
	DiffFile     string // review
	CoderSummary string // review
}

type AgentInfo struct {
	Name      string
	Strengths map[string]int
	Note      string
}

type PlannerInput struct {
	Goal     string
	Workdir  string
	IsGit    bool
	Listing  []string
	Inputs   []string
	Agents   []AgentInfo
	MaxSteps int
}

//go:embed prompts/*.md
var prompts embed.FS

var templates = template.Must(template.New("").Funcs(template.FuncMap{"imageHint": ImageHint}).ParseFS(prompts, "prompts/*.md"))

func Render(in Input) (string, error) {
	name := string(in.Kind)
	if in.Kind == api.KindCode && in.Round > 1 {
		name = "fix"
	}
	if name != "ask" && name != "code" && name != "fix" && name != "review" && name != "image" {
		return "", errors.New("请将步骤 kind 设为 ask/code/review/image")
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, name+".md", in); err != nil {
		return "", err
	}
	for _, upstream := range in.Upstream {
		summary := upstream.Summary
		if len(summary) > 8192 {
			summary = summary[:8192]
			for !utf8.ValidString(summary) {
				summary = summary[:len(summary)-1]
			}
		}
		fmt.Fprintf(&out, "\n上游步骤 %s（%s，%s）的结果：\n%s\n产物路径：\n%s\n变更文件：\n%s\n", upstream.StepID, upstream.Agent, upstream.Kind, summary, strings.Join(upstream.Artifacts, "\n"), strings.Join(upstream.Files, "\n"))
	}
	if len(in.InlineSchema) > 0 {
		fmt.Fprintf(&out, "\n最终回复只能是一个符合下面 JSON Schema 的 JSON 值，请直接输出 JSON：\n%s\n", in.InlineSchema)
	}
	return out.String(), nil
}

func RenderPlanner(in PlannerInput) (string, error) {
	var out bytes.Buffer
	err := templates.ExecuteTemplate(&out, "plan.md", in)
	return out.String(), err
}

func ExtractJSON(text string) (json.RawMessage, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if _, body, ok := strings.Cut(text, "\n"); ok {
			text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(body), "```"))
		}
	}
	if json.Valid([]byte(text)) {
		return json.RawMessage(text), nil
	}
	for i, ch := range text {
		if ch != '{' && ch != '[' {
			continue
		}
		var raw json.RawMessage
		if json.NewDecoder(strings.NewReader(text[i:])).Decode(&raw) == nil {
			return raw, nil
		}
	}
	return nil, errors.New("请直接返回符合 JSON Schema 的 JSON 内容")
}

func compile(schema []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return nil, fmt.Errorf("请提供合法的 JSON Schema：%w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("xcli-schema.json", doc); err != nil {
		return nil, err
	}
	compiled, err := compiler.Compile("xcli-schema.json")
	if err != nil {
		return nil, fmt.Errorf("请修正 JSON Schema：%w", err)
	}
	return compiled, nil
}

func CheckSchema(schema []byte) error { _, err := compile(schema); return err }

func Validate(schema []byte, doc json.RawMessage) error {
	compiled, err := compile(schema)
	if err != nil {
		return err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return fmt.Errorf("请返回合法的 JSON 内容：%w", err)
	}
	if err := compiled.Validate(value); err != nil {
		return fmt.Errorf("请按 JSON Schema 修正输出：%w", err)
	}
	return nil
}

func ImageHint(agent string) string {
	switch agent {
	case "codex":
		return "请使用你内置的图片生成工具（image_gen）生成图片。"
	case "antigravity":
		return "请调用内置的 image-generator 子代理生成图片（通过子代理调用，不要使用 --agent 参数）。"
	case "cursor":
		return "请使用内置的 GenerateImage 工具生成图片。"
	}
	return "请使用内置图片生成工具生成图片。"
}
