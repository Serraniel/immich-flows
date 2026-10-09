package steps

import (
	"fmt"
	"strings"
	"testing"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/flow"
)

func TestServiceFilename(t *testing.T) {
	src := `id: f
trigger: {on: manual}
steps:
  - id: parse
    use: regex
    with:
      input: '{{ asset.baseName }}'
      pattern: '^SERVICE-A_(?P<id>[^_]+)_(?P<tags>.+)$'
  - id: tags
    use: split
    if: steps.parse.matched
    with: {input: '{{ steps.parse.tags }}'}
  - use: set
    with:
      serviceId: '{{ steps.parse.id }}'
      tagValues: '{{ steps.tags.items | map("service-a/" + #) }}'
  - use: capture
`
	r := runFlow(t, src, map[string]any{"baseName": "SERVICE-A_123_cat_ dog__x"}, nil, engine.Deps{})
	if r.err != nil {
		t.Fatal(r.err)
	}
	if p := r.outputs["parse"]; p["matched"] != true || p["id"] != "123" || p["tags"] != "cat_ dog__x" {
		t.Errorf("parse: %v", p)
	}
	if it := r.outputs["tags"]["items"]; fmt.Sprint(it) != "[cat dog x]" {
		t.Errorf("split: %v", it)
	}
	if r.vars["serviceId"] != "123" || fmt.Sprint(r.vars["tagValues"]) != "[service-a/cat service-a/dog service-a/x]" {
		t.Errorf("vars: %v", r.vars)
	}
}

func TestRegexNoMatchSkipsDependentStep(t *testing.T) {
	src := `id: f
trigger: {on: manual}
steps:
  - {id: parse, use: regex, with: {input: 'HASH.jpg', pattern: '^SERVICE-A_', caseInsensitive: true}}
  - {id: tags, use: split, if: steps.parse.matched, with: {input: x}}
`
	r := runFlow(t, src, map[string]any{}, nil, engine.Deps{})
	if r.err != nil || r.outputs["parse"]["matched"] != false || r.outputs["tags"] != nil {
		t.Errorf("err=%v outputs=%v", r.err, r.outputs)
	}
}

func TestSplitOptions(t *testing.T) {
	src := `id: f
trigger: {on: manual}
steps:
  - {id: s, use: split, with: {input: 'a, b,,c', separator: ',', trim: false, skipEmpty: false}}
`
	r := runFlow(t, src, map[string]any{}, nil, engine.Deps{})
	if fmt.Sprintf("%q", r.outputs["s"]["items"]) != `["a" " b" "" "c"]` {
		t.Errorf("%v", r.outputs["s"])
	}
}

func TestTextValidation(t *testing.T) {
	reg := engine.NewRegistry()
	Register(reg)
	for src, want := range map[string]string{
		"steps: [{use: regex, with: {input: x, pattern: '('}}]": "pattern:",
		"steps: [{use: regex, with: {input: x}}]":               "with.pattern is required",
		"steps: [{use: split, with: {}}]":                       "with.input is required",
		"steps: [{use: set}]":                                   "at least one variable",
	} {
		f, err := flow.Parse([]byte("id: f\ntrigger: {on: manual}\n" + src))
		if err != nil {
			t.Fatal(err)
		}
		if err := reg.Validate(f); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", src, err)
		}
	}
}
