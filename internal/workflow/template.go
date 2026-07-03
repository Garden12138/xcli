package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

var templatePattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

type templateStep struct {
	Output     string
	OutputFile string
	SessionID  string
}

type templateContext struct {
	Vars  map[string]string
	Steps map[string]templateStep
}

func validateTemplateSyntax(value string, variables map[string]string, steps map[string]bool) error {
	matches := templatePattern.FindAllStringSubmatch(value, -1)
	for _, match := range matches {
		parts := strings.Split(match[1], ".")
		if len(parts) == 2 && parts[0] == "vars" {
			if _, ok := variables[parts[1]]; !ok {
				return fmt.Errorf("unknown template variable %q", match[1])
			}
			continue
		}
		if len(parts) == 3 && parts[0] == "steps" {
			if !steps[parts[1]] {
				return fmt.Errorf("step result %q must be declared earlier", parts[1])
			}
			switch parts[2] {
			case "output", "output_file", "session_id":
				continue
			}
		}
		return fmt.Errorf("unsupported template reference %q", match[1])
	}
	withoutMatches := templatePattern.ReplaceAllString(value, "")
	if strings.Contains(withoutMatches, "{{") || strings.Contains(withoutMatches, "}}") {
		return fmt.Errorf("invalid template syntax in %q", value)
	}
	return nil
}

func resolveTemplate(value string, context templateContext) (string, error) {
	var resolveErr error
	resolved := templatePattern.ReplaceAllStringFunc(value, func(match string) string {
		if resolveErr != nil {
			return ""
		}
		parts := strings.Split(templatePattern.FindStringSubmatch(match)[1], ".")
		if len(parts) == 2 && parts[0] == "vars" {
			result, ok := context.Vars[parts[1]]
			if !ok {
				resolveErr = fmt.Errorf("unknown template variable %q", strings.Join(parts, "."))
				return ""
			}
			return result
		}
		if len(parts) == 3 && parts[0] == "steps" {
			step, ok := context.Steps[parts[1]]
			if !ok {
				resolveErr = fmt.Errorf("step result %q is not available", parts[1])
				return ""
			}
			switch parts[2] {
			case "output":
				if len(step.Output) > MaxInlineOutput {
					resolveErr = fmt.Errorf("step %q output exceeds 128 KiB; reference {{ steps.%s.output_file }} instead", parts[1], parts[1])
					return ""
				}
				return step.Output
			case "output_file":
				if step.OutputFile == "" {
					resolveErr = fmt.Errorf("step %q has no output file", parts[1])
					return ""
				}
				return step.OutputFile
			case "session_id":
				if step.SessionID == "" {
					resolveErr = fmt.Errorf("step %q has no native session id", parts[1])
					return ""
				}
				return step.SessionID
			}
		}
		resolveErr = fmt.Errorf("unsupported template reference %q", strings.Join(parts, "."))
		return ""
	})
	if resolveErr != nil {
		return "", resolveErr
	}
	if strings.Contains(resolved, "{{") || strings.Contains(resolved, "}}") {
		return "", fmt.Errorf("invalid template syntax in %q", value)
	}
	return resolved, nil
}
