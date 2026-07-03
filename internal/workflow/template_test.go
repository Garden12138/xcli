package workflow

import (
	"strings"
	"testing"
)

func TestResolveTemplate(t *testing.T) {
	context := templateContext{
		Vars:  map[string]string{"requirement": "build it"},
		Steps: map[string]templateStep{"one": {Output: "done", OutputFile: "/tmp/output", SessionID: "session-1"}},
	}
	got, err := resolveTemplate("{{ vars.requirement }}: {{ steps.one.output }}", context)
	if err != nil {
		t.Fatal(err)
	}
	if got != "build it: done" {
		t.Fatalf("resolved = %q", got)
	}
}

func TestResolveTemplateRequiresOutputFileForLargeResult(t *testing.T) {
	context := templateContext{Vars: map[string]string{}, Steps: map[string]templateStep{
		"large": {Output: strings.Repeat("x", MaxInlineOutput+1), OutputFile: "/tmp/large"},
	}}
	if _, err := resolveTemplate("{{ steps.large.output }}", context); err == nil {
		t.Fatal("expected large-output error")
	}
	got, err := resolveTemplate("{{ steps.large.output_file }}", context)
	if err != nil || got != "/tmp/large" {
		t.Fatalf("output file = %q, %v", got, err)
	}
}

func TestValidateTemplateSyntaxRejectsUnknownOrForwardReferences(t *testing.T) {
	if err := validateTemplateSyntax("{{ vars.missing }}", map[string]string{}, map[string]bool{}); err == nil {
		t.Fatal("expected unknown-variable error")
	}
	if err := validateTemplateSyntax("{{ steps.future.output }}", map[string]string{}, map[string]bool{}); err == nil {
		t.Fatal("expected forward-step error")
	}
}
