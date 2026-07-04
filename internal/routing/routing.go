package routing

import (
	"fmt"
	"regexp"

	"github.com/Garden12138/xcli/internal/config"
)

const (
	SourceFlag       = "flag"
	SourcePositional = "positional"
	SourceRule       = "rule"
	SourceDefault    = "default"
)

type Decision struct {
	Agent  string `json:"agent"`
	Source string `json:"source"`
	Rule   string `json:"rule,omitempty"`
}

func Select(cfg config.Config, prompt string) (Decision, error) {
	for _, rule := range cfg.Routing.Rules {
		matched, err := regexp.MatchString(rule.PromptRegex, prompt)
		if err != nil {
			return Decision{}, fmt.Errorf("routing rule %q has invalid prompt_regex: %w", rule.Name, err)
		}
		if matched {
			return Decision{Agent: rule.Agent, Source: SourceRule, Rule: rule.Name}, nil
		}
	}
	if cfg.DefaultAgent != "" {
		return Decision{Agent: cfg.DefaultAgent, Source: SourceDefault}, nil
	}
	return Decision{}, fmt.Errorf("no routing rule matched and no default agent is configured")
}
