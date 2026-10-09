package steps

import (
	"fmt"
	"testing"

	"github.com/serraniel/immich-flows/internal/queue"
)

func TestMetadataSet(t *testing.T) {
	deps, f := fakeDeps(t)
	src := `id: f
trigger: {on: asset.uploaded}
steps:
  - id: h
    use: hash
    with: {source: original, algorithms: [md5]}
  - use: metadata.set
    with:
      key: immich-flows.hashes
      value: {originalMd5: '{{ steps.h.md5 }}', size: '{{ steps.h.size }}'}
`
	r := runFlowEvent(t, src, nil, map[string]any{"original": map[string]any{"md5": "abc", "size": 5}}, deps)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if got := fmt.Sprint(f.Metadata[runAsset]["immich-flows.hashes"]); got != "map[originalMd5:abc size:5]" {
		t.Errorf("metadata: %s", got)
	}

	r = runFlow(t, "id: f\ntrigger: {on: manual}\nsteps: [{use: metadata.set, with: {key: k, value: notanobject}}]\n", nil, deps)
	if !queue.IsPermanent(r.err) {
		t.Errorf("non-object value: %v", r.err)
	}
}
