package steps

import (
	"context"
	"sort"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/immich"
)

func init() {
	register(tagAssignStep())
	register(tagRemoveStep())
	register(tagReadStep())
}

var assetProps = map[string]any{
	"asset":  prop("string", "Asset ID, default: the run's asset"),
	"assets": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Asset IDs"},
}

func withAssets(props map[string]any) map[string]any {
	for k, v := range assetProps {
		props[k] = v
	}
	return props
}

var tagsProp = map[string]any{
	"description": "Tag values, hierarchical with /, e.g. service-a/cat",
	"oneOf":       []any{map[string]any{"type": "string"}, map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
}

func tagAssignStep() engine.StepType {
	return engine.StepType{
		Name:        "tag.assign",
		Description: "Assign tags, creating missing ones including parents.",
		Schema:      schema(withAssets(map[string]any{"tags": tagsProp}), "tags"),
		Validate:    func(with map[string]any) error { return requireKeys(with, "tags") },
		Run: func(ctx context.Context, sc *engine.StepContext, with map[string]any) (map[string]any, error) {
			values, err := tagValues(with, "tags")
			if err != nil {
				return nil, err
			}
			assets, err := assetIDs(sc, with)
			if err != nil {
				return nil, err
			}
			if len(values) == 0 {
				return map[string]any{"tags": []any{}, "count": 0}, nil
			}
			c, err := client(ctx, sc)
			if err != nil {
				return nil, err
			}
			up, err := c.UpsertTagsWithResponse(ctx, immich.TagUpsertDto{Tags: values})
			if err != nil {
				return nil, err
			}
			if err := check(up, up.Body); err != nil {
				return nil, err
			}
			ids := make([]openapi_types.UUID, 0, len(*up.JSON200))
			for _, t := range *up.JSON200 {
				ids = append(ids, t.Id)
			}
			res, err := c.BulkTagAssetsWithResponse(ctx, immich.TagBulkAssetsDto{TagIds: ids, AssetIds: assets})
			if err != nil {
				return nil, err
			}
			if err := check(res, res.Body); err != nil {
				return nil, err
			}
			return map[string]any{"tags": toAny(values), "count": len(values)}, nil
		},
	}
}

func tagRemoveStep() engine.StepType {
	return engine.StepType{
		Name:        "tag.remove",
		Description: "Remove tags from assets. The tags themselves are kept.",
		Schema:      schema(withAssets(map[string]any{"tags": tagsProp}), "tags"),
		Validate:    func(with map[string]any) error { return requireKeys(with, "tags") },
		Run: func(ctx context.Context, sc *engine.StepContext, with map[string]any) (map[string]any, error) {
			values, err := tagValues(with, "tags")
			if err != nil {
				return nil, err
			}
			assets, err := assetIDs(sc, with)
			if err != nil {
				return nil, err
			}
			c, err := client(ctx, sc)
			if err != nil {
				return nil, err
			}
			all, err := c.GetAllTagsWithResponse(ctx)
			if err != nil {
				return nil, err
			}
			if err := check(all, all.Body); err != nil {
				return nil, err
			}
			want := map[string]bool{}
			for _, v := range values {
				want[v] = true
			}
			removed := []any{}
			for _, t := range *all.JSON200 {
				if !want[t.Value] {
					continue
				}
				res, err := c.UntagAssetsWithResponse(ctx, t.Id, immich.BulkIdsDto{Ids: assets})
				if err != nil {
					return nil, err
				}
				if err := check(res, res.Body); err != nil {
					return nil, err
				}
				removed = append(removed, t.Value)
			}
			return map[string]any{"tags": removed, "count": len(removed)}, nil
		},
	}
}

func tagReadStep() engine.StepType {
	return engine.StepType{
		Name:        "tag.read",
		Description: "Read the tags of an asset.",
		Schema: schema(map[string]any{
			"asset":  assetProps["asset"],
			"prefix": prop("string", "Only tags starting with this, e.g. hashes/"),
		}),
		Run: func(ctx context.Context, sc *engine.StepContext, with map[string]any) (map[string]any, error) {
			ids, err := assetIDs(sc, map[string]any{"asset": with["asset"]})
			if err != nil {
				return nil, err
			}
			c, err := client(ctx, sc)
			if err != nil {
				return nil, err
			}
			values, err := assetTags(ctx, c, ids[0])
			if err != nil {
				return nil, err
			}
			prefix := str(with, "prefix", "")
			var tags, names []any
			sort.Strings(values)
			for _, v := range values {
				if !strings.HasPrefix(v, prefix) {
					continue
				}
				tags = append(tags, v)
				names = append(names, v[strings.LastIndex(v, "/")+1:])
			}
			if tags == nil {
				tags, names = []any{}, []any{}
			}
			return map[string]any{"tags": tags, "names": names, "count": len(tags)}, nil
		},
	}
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
