package steps

import (
	"os"
	"path/filepath"
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
