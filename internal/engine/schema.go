package engine

import (
	"encoding/json"

	"github.com/serraniel/immich-flows/internal/flow"
)

// FlowSchema returns the flow JSON Schema extended with the registered steps:
// use: lists the step names and with: is checked against each step's schema.
func (r *Registry) FlowSchema() (map[string]any, error) {
	raw, err := flow.Schema()
	if err != nil {
		return nil, err
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	step := s["$defs"].(map[string]any)["Step"].(map[string]any)
	props := step["properties"].(map[string]any)

	uses := []any{map[string]any{"const": flow.UseSwitch, "description": "Branch: runs the first case whose if is true"}}
	var conds []any
	for _, name := range r.Names() {
		t := r.types[name]
		uses = append(uses, map[string]any{"const": name, "description": t.Description})
		if t.Schema != nil {
			conds = append(conds, map[string]any{
				"if":   map[string]any{"properties": map[string]any{"use": map[string]any{"const": name}}, "required": []any{"use"}},
				"then": map[string]any{"properties": map[string]any{"with": t.Schema}},
			})
		}
	}
	props["use"] = map[string]any{"type": "string", "anyOf": uses}
	step["allOf"] = conds
	return s, nil
}
