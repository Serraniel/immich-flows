// Package steps contains the built-in flow steps. Each file registers its steps in init.
package steps

import (
	"fmt"

	"github.com/serraniel/immich-flows/internal/engine"
)

var all []engine.StepType

func register(t engine.StepType) { all = append(all, t) }

// Register adds all built-in steps to reg.
func Register(reg *engine.Registry) {
	for _, t := range all {
		reg.Register(t)
	}
}

// str returns with[key] as a string. Missing keys return def; non-strings are formatted.
func str(with map[string]any, key, def string) string {
	v, ok := with[key]
	if !ok || v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// boolean returns with[key] as a bool, def if missing.
func boolean(with map[string]any, key string, def bool) (bool, error) {
	v, ok := with[key]
	if !ok || v == nil {
		return def, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("with.%s must be a boolean", key)
	}
	return b, nil
}

func schema(props map[string]any, required ...string) map[string]any {
	req := make([]any, len(required))
	for i, r := range required {
		req[i] = r
	}
	return map[string]any{"type": "object", "properties": props, "required": req, "additionalProperties": false}
}

func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

// requireKeys checks that keys are present in a raw with: block.
func requireKeys(with map[string]any, keys ...string) error {
	for _, k := range keys {
		if _, ok := with[k]; !ok {
			return fmt.Errorf("with.%s is required", k)
		}
	}
	return nil
}

// strs returns with[key] as a list of strings. A single string is a one-element list.
func strs(with map[string]any, key string) ([]string, error) {
	switch v := with[key].(type) {
	case nil:
		return nil, nil
	case string:
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if x == nil {
				continue
			}
			out = append(out, fmt.Sprint(x))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("with.%s must be a string or a list", key)
	}
}
