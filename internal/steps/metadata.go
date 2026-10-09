package steps

import (
	"context"
	"errors"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/queue"
)

func init() { register(metadataSetStep()) }

func metadataSetStep() engine.StepType {
	return engine.StepType{
		Name:        "metadata.set",
		Description: "Write a JSON object to Immich asset metadata under key. Not shown in the Immich UI.",
		Schema: schema(map[string]any{
			"key":   prop("string", "Metadata key, e.g. immich-flows.hashes"),
			"value": map[string]any{"type": "object", "description": "Object to store; replaces the previous value"},
			"asset": assetProps["asset"],
		}, "key", "value"),
		Validate: func(with map[string]any) error { return requireKeys(with, "key", "value") },
		Run: func(ctx context.Context, sc *engine.StepContext, with map[string]any) (map[string]any, error) {
			key := str(with, "key", "")
			value, ok := with["value"].(map[string]any)
			if key == "" || !ok {
				return nil, queue.Permanent(errors.New("key must be a non-empty string and value an object"))
			}
			ids, err := assetIDs(sc, map[string]any{"asset": with["asset"]})
			if err != nil {
				return nil, err
			}
			c, err := client(ctx, sc)
			if err != nil {
				return nil, err
			}
			res, err := c.UpdateAssetMetadataWithResponse(ctx, ids[0], immich.AssetMetadataUpsertDto{
				Items: []immich.AssetMetadataUpsertItemDto{{Key: key, Value: value}},
			})
			if err != nil {
				return nil, err
			}
			if err := check(res, res.Body); err != nil {
				return nil, err
			}
			return map[string]any{"key": key, "value": value}, nil
		},
	}
}
