package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFlowSchema(t *testing.T) {
	s, err := NewRegistry().FlowSchema()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s["$defs"].(map[string]any)["Step"])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`{"const":"log","description":"Write a message to the run log"}`, `{"const":"switch"`, `"then":{"properties":{"with":{"additionalProperties":false`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
}
