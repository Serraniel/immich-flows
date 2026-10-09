package steps

import (
	"strings"
	"testing"

	"github.com/serraniel/immich-flows/internal/queue"
)

func TestTagMerge(t *testing.T) {
	deps, f := fakeDeps(t)
	a, b := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	f.Tag(a, "hashes/MD5/x")
	f.Tag(a, "people/anna")
	f.Tag(b, "hashes/MD5/y")
	f.Tag(runAsset, "keep")

	src := "id: f\ntrigger: {on: manual}\nsteps: [{id: m, use: tag.merge, with: {from: ['" + a + "', '" + b + "'], prefix: hashes/}}]\n"
	r := runFlow(t, src, nil, deps)
	if r.err != nil {
		t.Fatal(r.err)
	}
	assets, _ := f.State()
	if !strings.Contains(assets, runAsset+": hashes/MD5/x,hashes/MD5/y,keep") || r.outputs["m"]["count"] != float64(2) {
		t.Errorf("%s %v", assets, r.outputs["m"])
	}

	r = runFlow(t, "id: f\ntrigger: {on: manual}\nsteps: [{use: tag.merge, with: {from: [bad]}}]\n", nil, deps)
	if !queue.IsPermanent(r.err) {
		t.Errorf("invalid id: %v", r.err)
	}
}
