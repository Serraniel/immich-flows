// Package eval evaluates expressions (flow conditions) and {{ }} templates (step inputs).
package eval

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Env is the data visible to expressions.
type Env = map[string]any

const maxNodes = 2000

var cache sync.Map // source -> *vm.Program

func compile(src string) (*vm.Program, error) {
	if p, ok := cache.Load(src); ok {
		return p.(*vm.Program), nil
	}
	p, err := expr.Compile(src, expr.MaxNodes(maxNodes))
	if err != nil {
		return nil, fmt.Errorf("compile %q: %w", src, err)
	}
	cache.Store(src, p)
	return p, nil
}

// Check compiles src without running it.
func Check(src string) error {
	_, err := compile(src)
	return err
}

// Eval runs an expression.
func Eval(src string, env Env) (any, error) {
	p, err := compile(src)
	if err != nil {
		return nil, err
	}
	out, err := expr.Run(p, env)
	if err != nil {
		return nil, fmt.Errorf("eval %q: %w", src, err)
	}
	return out, nil
}

// Bool runs an expression that must return a bool. An empty expression is true.
func Bool(src string, env Env) (bool, error) {
	if strings.TrimSpace(src) == "" {
		return true, nil
	}
	v, err := Eval(src, env)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("eval %q: want bool, got %T", src, v)
	}
	return b, nil
}

// Render resolves templates in v: strings are rendered, maps and slices recursively.
// A string that is exactly one {{ expr }} yields the raw value (e.g. a list); otherwise the
// results are formatted into the string.
func Render(v any, env Env) (any, error) {
	switch t := v.(type) {
	case string:
		return renderString(t, env)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			r, err := Render(x, env)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(t))
		for i, x := range t {
			r, err := Render(x, env)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			// A template yielding a list inside a list is flattened: tags: [a, '{{ list }}'].
			if l, ok := r.([]any); ok && isSingleTemplate(x) {
				out = append(out, l...)
				continue
			}
			out = append(out, r)
		}
		return out, nil
	default:
		return v, nil
	}
}

// CheckTemplates compiles all templates in v.
func CheckTemplates(v any) error {
	var errs []error
	walkStrings(v, func(s string) {
		parts, err := split(s)
		if err != nil {
			errs = append(errs, err)
			return
		}
		for _, p := range parts {
			if p.expr {
				if err := Check(p.text); err != nil {
					errs = append(errs, err)
				}
			}
		}
	})
	return errors.Join(errs...)
}

func walkStrings(v any, fn func(string)) {
	switch t := v.(type) {
	case string:
		fn(t)
	case map[string]any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	case []any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	}
}

func isSingleTemplate(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	parts, err := split(s)
	return err == nil && len(parts) == 1 && parts[0].expr
}

func renderString(s string, env Env) (any, error) {
	parts, err := split(s)
	if err != nil {
		return nil, err
	}
	if len(parts) == 1 && parts[0].expr {
		v, err := Eval(parts[0].text, env)
		if err != nil {
			return nil, err
		}
		return normalize(v), nil
	}
	var b strings.Builder
	for _, p := range parts {
		if !p.expr {
			b.WriteString(p.text)
			continue
		}
		v, err := Eval(p.text, env)
		if err != nil {
			return nil, err
		}
		if v != nil {
			fmt.Fprint(&b, v)
		}
	}
	return b.String(), nil
}

// normalize converts typed slices to []any so callers handle one list type.
func normalize(v any) any {
	switch t := v.(type) {
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out
	default:
		return v
	}
}

type part struct {
	text string
	expr bool
}

// split cuts s into literal text and {{ expression }} parts. Braces inside quoted strings are ignored.
func split(s string) ([]part, error) {
	var parts []part
	for {
		start := strings.Index(s, "{{")
		if start < 0 {
			if s != "" {
				parts = append(parts, part{text: s})
			}
			return parts, nil
		}
		if start > 0 {
			parts = append(parts, part{text: s[:start]})
		}
		end := closing(s[start+2:])
		if end < 0 {
			return nil, fmt.Errorf("unclosed {{ in %q", s)
		}
		inner := strings.TrimSpace(s[start+2 : start+2+end])
		if inner == "" {
			return nil, fmt.Errorf("empty {{ }} in %q", s)
		}
		parts = append(parts, part{text: inner, expr: true})
		s = s[start+2+end+2:]
	}
}

func closing(s string) int {
	var quote byte
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote != '`' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '{':
			depth++
		case c == '}' && depth > 0:
			depth--
		case c == '}' && i+1 < len(s) && s[i+1] == '}':
			return i
		}
	}
	return -1
}
