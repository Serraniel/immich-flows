package steps

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/queue"
)

func init() {
	register(regexStep())
	register(splitStep())
	register(setStep())
}

func isTemplate(s string) bool { return strings.Contains(s, "{{") }

func regexStep() engine.StepType {
	return engine.StepType{
		Name:        "regex",
		Description: "Match input against a regular expression (RE2). Named groups become outputs.",
		Schema: schema(map[string]any{
			"input":           prop("string", "Text to match, usually a template"),
			"pattern":         prop("string", "RE2 pattern, e.g. ^(?P<service>[^_]+)_"),
			"caseInsensitive": prop("boolean", "Default false"),
		}, "input", "pattern"),
		Validate: func(with map[string]any) error {
			if err := requireKeys(with, "input", "pattern"); err != nil {
				return err
			}
			if p := str(with, "pattern", ""); !isTemplate(p) {
				if _, err := regexp.Compile(p); err != nil {
					return fmt.Errorf("pattern: %w", err)
				}
			}
			return nil
		},
		Run: func(_ context.Context, _ *engine.StepContext, with map[string]any) (map[string]any, error) {
			pattern := str(with, "pattern", "")
			ci, err := boolean(with, "caseInsensitive", false)
			if err != nil {
				return nil, queue.Permanent(err)
			}
			if ci {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, queue.Permanent(fmt.Errorf("pattern: %w", err))
			}
			m := re.FindStringSubmatch(str(with, "input", ""))
			out := map[string]any{"matched": m != nil}
			if m == nil {
				return out, nil
			}
			groups := make([]any, len(m))
			for i, g := range m {
				groups[i] = g
			}
			out["match"], out["groups"] = m[0], groups
			for i, name := range re.SubexpNames() {
				if name != "" {
					out[name] = m[i]
				}
			}
			return out, nil
		},
	}
}

func splitStep() engine.StepType {
	return engine.StepType{
		Name:        "split",
		Description: "Split input into items.",
		Schema: schema(map[string]any{
			"input":     prop("string", "Text to split"),
			"separator": prop("string", "Default _"),
			"trim":      prop("boolean", "Trim spaces around items, default true"),
			"skipEmpty": prop("boolean", "Drop empty items, default true"),
		}, "input"),
		Validate: func(with map[string]any) error { return requireKeys(with, "input") },
		Run: func(_ context.Context, _ *engine.StepContext, with map[string]any) (map[string]any, error) {
			trim, err := boolean(with, "trim", true)
			if err != nil {
				return nil, queue.Permanent(err)
			}
			skip, err := boolean(with, "skipEmpty", true)
			if err != nil {
				return nil, queue.Permanent(err)
			}
			sep := str(with, "separator", "_")
			if sep == "" {
				return nil, queue.Permanent(fmt.Errorf("separator must not be empty"))
			}
			items := []any{}
			for _, p := range strings.Split(str(with, "input", ""), sep) {
				if trim {
					p = strings.TrimSpace(p)
				}
				if skip && p == "" {
					continue
				}
				items = append(items, p)
			}
			return map[string]any{"items": items, "count": len(items)}, nil
		},
	}
}

func setStep() engine.StepType {
	return engine.StepType{
		Name:        "set",
		Description: "Set variables, available as vars.<name> in later steps.",
		Schema:      map[string]any{"type": "object", "additionalProperties": true},
		Validate: func(with map[string]any) error {
			if len(with) == 0 {
				return fmt.Errorf("set needs at least one variable")
			}
			return nil
		},
		Run: func(_ context.Context, sc *engine.StepContext, with map[string]any) (map[string]any, error) {
			vars, _ := sc.Env["vars"].(map[string]any)
			if vars == nil {
				return nil, queue.Permanent(fmt.Errorf("no vars in environment"))
			}
			for k, v := range with {
				vars[k] = v
			}
			return with, nil
		},
	}
}
