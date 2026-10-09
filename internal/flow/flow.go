// Package flow defines the flow document format and its validation.
package flow

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Triggers lists valid values for trigger.on.
var Triggers = []string{
	"asset.uploaded",
	"asset.trashed",
	"asset.deleted",
	"asset.restored",
	"duplicates.resolved",
	"tag.assigned",
	"tag.removed",
	"manual",
}

// UseSwitch is the built-in branching step.
const UseSwitch = "switch"

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Flow is a trigger and the steps it runs.
type Flow struct {
	ID          string         `yaml:"id" json:"id" jsonschema:"pattern=^[a-z0-9][a-z0-9_-]*$"`
	Name        string         `yaml:"name,omitempty" json:"name,omitempty"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Enabled     *bool          `yaml:"enabled,omitempty" json:"enabled,omitempty" jsonschema:"default=true"`
	Trigger     Trigger        `yaml:"trigger" json:"trigger"`
	Steps       []Step         `yaml:"steps" json:"steps" jsonschema:"minItems=1"`
	UI          map[string]any `yaml:"ui,omitempty" json:"ui,omitempty" jsonschema:"description=Editor state (node positions). Ignored by the engine."`

	pos pos
}

// Trigger selects the event a flow reacts to.
type Trigger struct {
	On string `yaml:"on" json:"on"`
	If string `yaml:"if,omitempty" json:"if,omitempty" jsonschema:"description=Expression; the flow runs only if true"`

	pos pos
}

// Step is one unit of work. Use "switch" with Cases to branch.
type Step struct {
	ID              string         `yaml:"id,omitempty" json:"id,omitempty" jsonschema:"pattern=^[a-z0-9][a-z0-9_-]*$,description=Required to reference outputs (steps.<id>)"`
	Name            string         `yaml:"name,omitempty" json:"name,omitempty"`
	Use             string         `yaml:"use" json:"use"`
	If              string         `yaml:"if,omitempty" json:"if,omitempty" jsonschema:"description=Expression; the step is skipped if false"`
	With            map[string]any `yaml:"with,omitempty" json:"with,omitempty"`
	Cases           []Case         `yaml:"cases,omitempty" json:"cases,omitempty" jsonschema:"description=Only for use: switch. First matching case runs."`
	ContinueOnError bool           `yaml:"continueOnError,omitempty" json:"continueOnError,omitempty"`

	pos pos
}

// Case is a branch of a switch step. An empty If matches always (default branch).
type Case struct {
	If    string `yaml:"if,omitempty" json:"if,omitempty"`
	Steps []Step `yaml:"steps" json:"steps"`

	pos pos
}

// IsEnabled reports whether the flow is enabled (default true).
func (f *Flow) IsEnabled() bool { return f.Enabled == nil || *f.Enabled }

type pos struct{ line, col int }

func (p pos) String() string { return fmt.Sprintf("%d:%d", p.line, p.col) }

// Error is a validation problem at a position in the source.
type Error struct {
	Line, Col int
	Msg       string
}

func (e Error) Error() string { return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg) }

// Errors is a list of validation problems.
type Errors []Error

func (e Errors) Error() string {
	s := make([]string, len(e))
	for i, err := range e {
		s[i] = err.Error()
	}
	return strings.Join(s, "\n")
}

// Parse decodes and validates a single flow document.
func Parse(data []byte) (*Flow, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var f Flow
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, Errors{{Line: 1, Col: 1, Msg: "empty document"}}
		}
		return nil, yamlErrors(err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, Errors{{Line: 1, Col: 1, Msg: "expected exactly one YAML document"}}
	}
	if errs := f.validate(); len(errs) > 0 {
		return nil, errs
	}
	return &f, nil
}

// Marshal encodes a flow as YAML.
func Marshal(f *Flow) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}

var lineRe = regexp.MustCompile(`^(?:yaml: )?line (\d+): (.*)$`)

func yamlErrors(err error) error {
	var pe Error
	if errors.As(err, &pe) {
		return Errors{pe}
	}
	var te *yaml.TypeError
	msgs := []string{err.Error()}
	if errors.As(err, &te) {
		msgs = te.Errors
	}
	var out Errors
	for _, m := range msgs {
		if sub := lineRe.FindStringSubmatch(m); sub != nil {
			var line int
			_, _ = fmt.Sscan(sub[1], &line)
			out = append(out, Error{Line: line, Col: 1, Msg: sub[2]})
			continue
		}
		out = append(out, Error{Line: 1, Col: 1, Msg: m})
	}
	return out
}

func (f *Flow) validate() Errors {
	var errs Errors
	add := func(p pos, format string, a ...any) {
		errs = append(errs, Error{Line: p.line, Col: p.col, Msg: fmt.Sprintf(format, a...)})
	}

	if !idPattern.MatchString(f.ID) {
		add(f.pos, "id %q must match %s", f.ID, idPattern)
	}
	if !slices.Contains(Triggers, f.Trigger.On) {
		add(f.Trigger.pos, "trigger.on %q unknown, valid: %s", f.Trigger.On, strings.Join(Triggers, ", "))
	}
	if len(f.Steps) == 0 {
		add(f.pos, "steps must not be empty")
	}

	ids := map[string]pos{}
	var walk func([]Step)
	walk = func(steps []Step) {
		for _, s := range steps {
			if s.ID != "" {
				if !idPattern.MatchString(s.ID) {
					add(s.pos, "step id %q must match %s", s.ID, idPattern)
				}
				if prev, dup := ids[s.ID]; dup {
					add(s.pos, "step id %q already used at %s", s.ID, prev)
				} else {
					ids[s.ID] = s.pos
				}
			}
			switch {
			case s.Use == "":
				add(s.pos, "step needs use")
			case s.Use == UseSwitch && len(s.Cases) == 0:
				add(s.pos, "switch needs cases")
			case s.Use != UseSwitch && len(s.Cases) > 0:
				add(s.pos, "cases only allowed with use: switch")
			}
			for i, c := range s.Cases {
				if c.If == "" && i != len(s.Cases)-1 {
					add(c.pos, "case without if must be last")
				}
				if len(c.Steps) == 0 {
					add(c.pos, "case needs steps")
				}
				walk(c.Steps)
			}
		}
	}
	walk(f.Steps)

	sort.SliceStable(errs, func(i, j int) bool {
		if errs[i].Line != errs[j].Line {
			return errs[i].Line < errs[j].Line
		}
		return errs[i].Col < errs[j].Col
	})
	return errs
}
