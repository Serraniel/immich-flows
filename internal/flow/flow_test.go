package flow

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestParseExample(t *testing.T) {
	data, err := os.ReadFile("../../examples/flows/service-tags.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.ID != "service-tags" || f.Trigger.On != "asset.uploaded" || !f.IsEnabled() {
		t.Errorf("unexpected flow: %+v", f)
	}
	if sw := f.Steps[2]; sw.Use != UseSwitch || len(sw.Cases) != 2 || sw.Cases[0].Steps[0].ID != "a" {
		t.Errorf("switch not decoded: %+v", sw)
	}
}

func TestRoundTrip(t *testing.T) {
	src := `id: x
trigger:
  on: manual
steps:
  - id: h
    use: hash
    with:
      algorithms: [md5]
ui:
  nodes:
    h: {x: 10, y: 20}
`
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	g, err := Parse(out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if g.Steps[0].With["algorithms"].([]any)[0] != "md5" || g.UI["nodes"] == nil {
		t.Errorf("lost data:\n%s", out)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"empty":         {``, "1:1: empty document"},
		"unknown field": {"id: x\ntrigger: {on: manual}\nstep: []\n", `3:1: unknown field "step"`},
		"unknown step field": {"id: x\ntrigger: {on: manual}\nsteps:\n  - use: log\n    input: {}\n",
			`5:5: unknown field "input"`},
		"bad id":      {"id: X Y\ntrigger: {on: manual}\nsteps: [{use: log}]\n", `id "X Y" must match`},
		"bad trigger": {"id: x\ntrigger: {on: nope}\nsteps: [{use: log}]\n", `2:10: trigger.on "nope" unknown`},
		"no steps":    {"id: x\ntrigger: {on: manual}\nsteps: []\n", "steps must not be empty"},
		"no use":      {"id: x\ntrigger: {on: manual}\nsteps:\n  - id: a\n", "4:5: step needs use"},
		"dup id": {"id: x\ntrigger: {on: manual}\nsteps:\n  - {id: a, use: log}\n  - use: switch\n    cases:\n      - steps: [{id: a, use: log}]\n",
			`7:17: step id "a" already used at 4:5`},
		"default not last": {"id: x\ntrigger: {on: manual}\nsteps:\n  - use: switch\n    cases:\n      - steps: [{use: log}]\n      - if: 'true'\n        steps: [{use: log}]\n",
			"6:9: case without if must be last"},
		"cases without switch": {"id: x\ntrigger: {on: manual}\nsteps:\n  - use: log\n    cases: [{steps: [{use: log}]}]\n",
			"cases only allowed with use: switch"},
		"type error":    {"id: x\ntrigger: {on: manual}\nsteps: nope\n", "3:"},
		"two documents": {"id: x\n---\nid: y\n", "exactly one YAML document"},
		"syntax error":  {"id: [x\n", "1:"},
		"not a mapping": {"- a\n", "expected a mapping"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(c.src))
			var errs Errors
			if !errors.As(err, &errs) {
				t.Fatalf("want Errors, got %T %v", err, err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %q, want %q", err, c.want)
			}
		})
	}
}

func TestSchema(t *testing.T) {
	b, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"duplicates.resolved"`) || !strings.Contains(string(b), `"continueOnError"`) {
		t.Errorf("schema incomplete:\n%s", b)
	}
}

func TestScheduleTrigger(t *testing.T) {
	ok := "id: s\ntrigger: {on: schedule, cron: '0 3 * * *'}\nsteps: [{use: log}]\n"
	if f, err := Parse([]byte(ok)); err != nil || f.Trigger.Cron != "0 3 * * *" {
		t.Fatalf("%v %+v", err, f)
	}
	for name, src := range map[string]string{
		"missing cron":  "id: s\ntrigger: {on: schedule}\nsteps: [{use: log}]\n",
		"bad cron":      "id: s\ntrigger: {on: schedule, cron: '61 * * * *'}\nsteps: [{use: log}]\n",
		"cron on other": "id: s\ntrigger: {on: manual, cron: '@daily'}\nsteps: [{use: log}]\n",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
