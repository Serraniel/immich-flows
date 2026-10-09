package engine

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/serraniel/immich-flows/internal/immich"
)

// ClientFunc returns an Immich client acting as the user.
type ClientFunc func(ctx context.Context, userID string) (*immich.ClientWithResponses, error)

// ImmichAssets returns a Deps.LoadAsset that reads the asset from Immich. Besides Immich's
// asset fields it adds baseName (filename without extension) and extension (lowercase, no dot).
func ImmichAssets(clients ClientFunc) func(ctx context.Context, userID, assetID string) (map[string]any, error) {
	return func(ctx context.Context, userID, assetID string) (map[string]any, error) {
		c, err := clients(ctx, userID)
		if err != nil {
			return nil, err
		}
		var id openapi_types.UUID
		if err := id.UnmarshalText([]byte(assetID)); err != nil {
			return nil, err
		}
		res, err := c.GetAssetInfoWithResponse(ctx, id, nil)
		if err != nil {
			return nil, err
		}
		if err := immich.Check(res, res.Body); err != nil {
			return nil, err
		}
		return assetEnv(res.Body)
	}
}

func assetEnv(body []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	name, _ := m["originalFileName"].(string)
	ext := path.Ext(name)
	m["baseName"] = strings.TrimSuffix(name, ext)
	m["extension"] = strings.ToLower(strings.TrimPrefix(ext, "."))
	return m, nil
}
