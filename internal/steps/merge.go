package steps

import (
	"context"
	"sort"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/queue"
)

func init() { register(tagMergeStep()) }

func tagMergeStep() engine.StepType {
	stringList := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	return engine.StepType{
		Name:        "tag.merge",
		Description: "Apply the union of the tags of the from assets to the to assets.",
		Schema: schema(map[string]any{
			"from":   stringList,
			"to":     stringList,
			"prefix": prop("string", "Only merge tags starting with this"),
		}, "from"),
		Validate: func(with map[string]any) error { return requireKeys(with, "from") },
		Run: func(ctx context.Context, sc *engine.StepContext, with map[string]any) (map[string]any, error) {
			from, err := strs(with, "from")
			if err != nil {
				return nil, queue.Permanent(err)
			}
			to, err := assetIDs(sc, map[string]any{"assets": with["to"]})
			if err != nil {
				return nil, err
			}
			sources, err := assetIDs(sc, map[string]any{"assets": toAny(from), "asset": ""})
			if err != nil {
				return nil, err
			}
			c, err := client(ctx, sc)
			if err != nil {
				return nil, err
			}
			prefix := str(with, "prefix", "")
			union := map[string]bool{}
			for _, id := range sources {
				values, err := assetTags(ctx, c, id)
				if err != nil {
					return nil, err
				}
				for _, v := range values {
					if strings.HasPrefix(v, prefix) {
						union[v] = true
					}
				}
			}
			if len(union) == 0 {
				return map[string]any{"tags": []any{}, "count": 0}, nil
			}
			values := make([]string, 0, len(union))
			for v := range union {
				values = append(values, v)
			}
			sort.Strings(values)

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
			res, err := c.BulkTagAssetsWithResponse(ctx, immich.TagBulkAssetsDto{TagIds: ids, AssetIds: to})
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
