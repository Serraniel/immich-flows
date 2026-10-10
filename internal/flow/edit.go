package flow

import (
	"bytes"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// SetEnabled sets the top-level enabled field of a flow document, keeping comments.
func SetEnabled(data []byte, enabled bool) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, yamlErrors(err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, Errors{{Line: 1, Col: 1, Msg: "expected a mapping"}}
	}
	m := doc.Content[0]
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(enabled)}
	set := false
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == "enabled" {
			m.Content[i+1] = val
			set = true
		}
	}
	if !set {
		// After id/name/description, before trigger.
		at := 0
		for i := 0; i < len(m.Content); i += 2 {
			switch m.Content[i].Value {
			case "id", "name", "description":
				at = i + 2
			}
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "enabled"}
		m.Content = append(m.Content[:at], append([]*yaml.Node{key, val}, m.Content[at:]...)...)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
