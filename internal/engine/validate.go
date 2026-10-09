package engine

import (
	"fmt"

	"github.com/serraniel/immich-flows/internal/eval"
	"github.com/serraniel/immich-flows/internal/flow"
)

// Validate checks that a parsed flow only uses known steps and valid expressions.
func (r *Registry) Validate(f *flow.Flow) error {
	var errs flow.Errors
	if err := eval.Check(f.Trigger.If); f.Trigger.If != "" && err != nil {
		errs = append(errs, f.Trigger.ErrorAt("trigger.if: %v", err))
	}
	var walk func([]flow.Step)
	walk = func(steps []flow.Step) {
		for i := range steps {
			s := &steps[i]
			if s.If != "" {
				if err := eval.Check(s.If); err != nil {
					errs = append(errs, s.ErrorAt("if: %v", err))
				}
			}
			if s.Use == flow.UseSwitch {
				for _, c := range s.Cases {
					if c.If != "" {
						if err := eval.Check(c.If); err != nil {
							errs = append(errs, s.ErrorAt("case if: %v", err))
						}
					}
					walk(c.Steps)
				}
				continue
			}
			t, ok := r.Get(s.Use)
			if !ok {
				errs = append(errs, s.ErrorAt("unknown step %q", s.Use))
				continue
			}
			if err := eval.CheckTemplates(anyMap(s.With)); err != nil {
				errs = append(errs, s.ErrorAt("with: %v", err))
			}
			if t.Validate != nil {
				if err := t.Validate(s.With); err != nil {
					errs = append(errs, s.ErrorAt("%s: %v", s.Use, err))
				}
			}
		}
	}
	walk(f.Steps)
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func anyMap(m map[string]any) any {
	if m == nil {
		return nil
	}
	return m
}

// require returns an error if any key is missing from with.
func require(with map[string]any, keys ...string) error {
	for _, k := range keys {
		if _, ok := with[k]; !ok {
			return fmt.Errorf("with.%s is required", k)
		}
	}
	return nil
}
