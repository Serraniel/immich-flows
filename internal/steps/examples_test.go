package steps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/flow"
)

// Example and e2e flows must stay valid.
func TestExampleFlowsValidate(t *testing.T) {
	reg := engine.NewRegistry()
	Register(reg)
	files, _ := filepath.Glob("../../examples/flows/*.yaml")
	e2e, _ := filepath.Glob("../../e2e/flows/*.yaml")
	for _, p := range append(files, e2e...) {
		if filepath.Base(p) == "invalid.yaml" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		f, err := flow.Parse(data)
		if err == nil {
			err = reg.Validate(f)
		}
		if err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if len(files) == 0 {
		t.Error("no example flows found")
	}
}

func TestSchemaFile(t *testing.T) {
	reg := engine.NewRegistry()
	Register(reg)
	s, err := reg.FlowSchema()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("../../docs/flow.schema.json")
	if err != nil || string(committed) != string(b)+"\n" {
		t.Error("docs/flow.schema.json is stale, run go generate ./...")
	}
}

// Complete flows (```yaml blocks starting with id:) in the docs must stay valid.
func TestDocFlowsValidate(t *testing.T) {
	reg := engine.NewRegistry()
	Register(reg)
	block := regexp.MustCompile("(?s)```yaml\n(id: .*?)```")
	n := 0
	for _, p := range []string{"../../docs/cookbook.md", "../../docs/flows.md"} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range block.FindAllSubmatch(data, -1) {
			n++
			f, err := flow.Parse(m[1])
			if err == nil {
				err = reg.Validate(f)
			}
			if err != nil {
				t.Errorf("%s: %v\n%s", p, err, m[1])
			}
		}
	}
	if n < 5 {
		t.Errorf("found %d doc flows", n)
	}
}
