package flow

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

//go:generate sh -c "go run ../../cmd/immich-flows schema > ../../docs/flow.schema.json"

// Schema returns the JSON Schema of a flow document.
func Schema() ([]byte, error) {
	r := &jsonschema.Reflector{ExpandedStruct: true}
	s := r.Reflect(&Flow{})
	s.ID = "https://github.com/serraniel/immich-flows/raw/main/docs/flow.schema.json"
	s.Title = "immich-flows flow"
	s.Definitions["Trigger"].Properties.Value("on").Enum = toAny(Triggers)
	return json.MarshalIndent(s, "", "  ")
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
