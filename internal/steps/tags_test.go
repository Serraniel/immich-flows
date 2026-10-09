package steps

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/immichtest"
	"github.com/serraniel/immich-flows/internal/queue"
)

const runAsset = "11111111-1111-4111-8111-111111111111"

func fakeDeps(t *testing.T) (engine.Deps, *immichtest.Fake) {
	t.Helper()
	f := immichtest.New()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return engine.Deps{Immich: func(context.Context, string) (*immich.ClientWithResponses, error) {
		return immich.New(u, immich.APIKey("k"), nil)
	}}, f
}

func TestServiceFlowTags(t *testing.T) {
	deps, f := fakeDeps(t)
	src := `id: f
trigger: {on: manual}
steps:
  - id: parse
    use: regex
    with: {input: '{{ asset.baseName }}', pattern: '^SERVICE-A_(?P<id>[^_]+)_(?P<tags>.+)$'}
  - id: assign
    use: tag.assign
    if: steps.parse.matched
    with:
      tags: ['service-a', ' service-a / id / {{ steps.parse.id }} ', '{{ split(steps.parse.tags, "_") | map("service-a/" + #) }}', '']
  - id: read
    use: tag.read
    with: {prefix: service-a/}
`
	r := runFlow(t, src, map[string]any{"baseName": "SERVICE-A_7_cat_dog"}, deps)
	if r.err != nil {
		t.Fatal(r.err)
	}
	assets, _ := f.State()
	want := runAsset + ": service-a,service-a/cat,service-a/dog,service-a/id/7"
	if assets != want {
		t.Errorf("assets: %s\nwant    %s", assets, want)
	}
	if fmt.Sprint(r.outputs["read"]["names"]) != "[cat dog 7]" || r.outputs["assign"]["count"] != float64(4) {
		t.Errorf("outputs: %v", r.outputs)
	}
}

func TestTagRemove(t *testing.T) {
	deps, f := fakeDeps(t)
	f.Tag(runAsset, "keep")
	f.Tag(runAsset, "drop/me")
	f.Tag("22222222-2222-4222-8222-222222222222", "drop/me")
	src := `id: f
trigger: {on: manual}
steps:
  - {id: rm, use: tag.remove, with: {tags: [drop/me, missing]}}
`
	r := runFlow(t, src, nil, deps)
	if r.err != nil {
		t.Fatal(r.err)
	}
	assets, values := f.State()
	if !strings.Contains(assets, runAsset+": keep") || !strings.Contains(assets, "22222222-2222-4222-8222-222222222222: drop/me") ||
		!strings.Contains(values, "drop/me") || r.outputs["rm"]["count"] != float64(1) {
		t.Errorf("%s | %s | %v", assets, values, r.outputs["rm"])
	}
}

func TestTagAssignExplicitAssetsAndErrors(t *testing.T) {
	deps, f := fakeDeps(t)
	other := "33333333-3333-4333-8333-333333333333"
	r := runFlow(t, "id: f\ntrigger: {on: manual}\nsteps: [{use: tag.assign, with: {tags: x, assets: ['"+other+"', '"+other+"']}}]\n", nil, deps)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if assets, _ := f.State(); assets != other+": x" {
		t.Errorf("%s", assets)
	}

	r = runFlow(t, "id: f\ntrigger: {on: manual}\nsteps: [{use: tag.assign, with: {tags: x, asset: nope}}]\n", nil, deps)
	if !queue.IsPermanent(r.err) {
		t.Errorf("invalid id must be permanent: %v", r.err)
	}
	r = runFlow(t, "id: f\ntrigger: {on: manual}\nsteps: [{use: tag.assign, with: {tags: x}}]\n", nil, engine.Deps{})
	if !queue.IsPermanent(r.err) {
		t.Errorf("missing client must be permanent: %v", r.err)
	}
}

func TestTagValue(t *testing.T) {
	for in, want := range map[string]string{" a / b /": "a/b", "//": "", "x": "x"} {
		if got := tagValue(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}
