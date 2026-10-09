package flow

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Custom unmarshalers record positions and reject unknown keys.

func strict(n *yaml.Node, allowed ...string) error {
	if n.Kind != yaml.MappingNode {
		return Error{Line: n.Line, Col: n.Column, Msg: "expected a mapping"}
	}
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		ok := false
		for _, a := range allowed {
			if k.Value == a {
				ok = true
				break
			}
		}
		if !ok {
			return Error{Line: k.Line, Col: k.Column, Msg: fmt.Sprintf("unknown field %q", k.Value)}
		}
	}
	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (f *Flow) UnmarshalYAML(n *yaml.Node) error {
	if err := strict(n, "id", "name", "description", "enabled", "trigger", "steps", "ui"); err != nil {
		return err
	}
	type plain Flow
	if err := n.Decode((*plain)(f)); err != nil {
		return err
	}
	f.pos = pos{n.Line, n.Column}
	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (t *Trigger) UnmarshalYAML(n *yaml.Node) error {
	if err := strict(n, "on", "if"); err != nil {
		return err
	}
	type plain Trigger
	if err := n.Decode((*plain)(t)); err != nil {
		return err
	}
	t.pos = pos{n.Line, n.Column}
	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Step) UnmarshalYAML(n *yaml.Node) error {
	if err := strict(n, "id", "name", "use", "if", "with", "cases", "continueOnError"); err != nil {
		return err
	}
	type plain Step
	if err := n.Decode((*plain)(s)); err != nil {
		return err
	}
	s.pos = pos{n.Line, n.Column}
	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (c *Case) UnmarshalYAML(n *yaml.Node) error {
	if err := strict(n, "if", "steps"); err != nil {
		return err
	}
	type plain Case
	if err := n.Decode((*plain)(c)); err != nil {
		return err
	}
	c.pos = pos{n.Line, n.Column}
	return nil
}
