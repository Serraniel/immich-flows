// Package engine validates and executes flows.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
)

// StepFunc runs a step with its rendered input and returns its output.
type StepFunc func(ctx context.Context, sc *StepContext, with map[string]any) (map[string]any, error)

// StepType is a registered step.
type StepType struct {
	Name        string
	Description string
	// Schema is the JSON Schema of the step's with: block (used by validation and editors).
	Schema map[string]any
	// Validate checks the raw (unrendered) with: block at load time. Optional.
	Validate func(with map[string]any) error
	Run      StepFunc
}

// StepContext gives a step access to the run.
type StepContext struct {
	FlowID  string
	UserID  string
	AssetID string
	Env     map[string]any // read-only view of trigger, asset, steps, vars
	Log     *slog.Logger
	Deps    Deps
}

// Registry holds step types by name.
type Registry struct {
	types map[string]StepType
}

// NewRegistry returns a registry with the built-in steps.
func NewRegistry() *Registry {
	r := &Registry{types: map[string]StepType{}}
	r.Register(logStep())
	return r
}

// Register adds a step type. It panics on duplicates; registration happens at startup.
func (r *Registry) Register(t StepType) {
	if _, dup := r.types[t.Name]; dup {
		panic(fmt.Sprintf("step %q registered twice", t.Name))
	}
	r.types[t.Name] = t
}

// Get returns the step type by name.
func (r *Registry) Get(name string) (StepType, bool) {
	t, ok := r.types[name]
	return t, ok
}

// Names returns all step names, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.types))
	for n := range r.types {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func logStep() StepType {
	return StepType{
		Name:        "log",
		Description: "Write a message to the run log",
		Schema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"message": map[string]any{"type": "string"}},
			"required":             []any{"message"},
			"additionalProperties": false,
		},
		Validate: func(with map[string]any) error { return require(with, "message") },
		Run: func(_ context.Context, sc *StepContext, with map[string]any) (map[string]any, error) {
			msg := fmt.Sprint(with["message"])
			sc.Log.Info(msg)
			return map[string]any{"message": msg}, nil
		},
	}
}
