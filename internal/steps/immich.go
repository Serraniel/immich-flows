package steps

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/queue"
)

func client(ctx context.Context, sc *engine.StepContext) (*immich.ClientWithResponses, error) {
	if sc.Deps.Immich == nil {
		return nil, queue.Permanent(errors.New("no Immich client configured"))
	}
	return sc.Deps.Immich(ctx, sc.UserID)
}

// assetIDs returns with.assets (list) or with.asset, defaulting to the run's asset.
func assetIDs(sc *engine.StepContext, with map[string]any) ([]openapi_types.UUID, error) {
	ids, err := strs(with, "assets")
	if err != nil {
		return nil, queue.Permanent(err)
	}
	if len(ids) == 0 {
		if a := str(with, "asset", sc.AssetID); a != "" {
			ids = []string{a}
		}
	}
	if len(ids) == 0 {
		return nil, queue.Permanent(errors.New("no asset: the trigger has none, set with.asset or with.assets"))
	}
	out := make([]openapi_types.UUID, 0, len(ids))
	seen := map[string]bool{}
	for _, s := range ids {
		if seen[s] {
			continue
		}
		seen[s] = true
		var u openapi_types.UUID
		if err := u.UnmarshalText([]byte(s)); err != nil {
			return nil, queue.Permanent(fmt.Errorf("invalid asset id %q", s))
		}
		out = append(out, u)
	}
	return out, nil
}

// tagValue normalizes a hierarchical tag value: trims segments and drops empty ones.
func tagValue(v string) string {
	parts := strings.Split(v, "/")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "/")
}

// tagValues normalizes and de-duplicates with[key].
func tagValues(with map[string]any, key string) ([]string, error) {
	raw, err := strs(with, key)
	if err != nil {
		return nil, queue.Permanent(err)
	}
	var out []string
	seen := map[string]bool{}
	for _, r := range raw {
		if v := tagValue(r); v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out, nil
}

// permanentIfClientError marks 4xx Immich errors (except 429) as not retryable.
func permanentIfClientError(err error) error {
	var ie *immich.Error
	if errors.As(err, &ie) && ie.Status >= 400 && ie.Status < 500 && ie.Status != 429 {
		return queue.Permanent(err)
	}
	return err
}

func check(res interface{ StatusCode() int }, body []byte) error {
	return permanentIfClientError(immich.Check(res, body))
}

// assetTags returns the tag values directly on an asset.
func assetTags(ctx context.Context, c *immich.ClientWithResponses, id openapi_types.UUID) ([]string, error) {
	res, err := c.GetAssetInfoWithResponse(ctx, id, nil)
	if err != nil {
		return nil, err
	}
	if err := check(res, res.Body); err != nil {
		return nil, err
	}
	var out []string
	if res.JSON200.Tags != nil {
		for _, t := range *res.JSON200.Tags {
			out = append(out, t.Value)
		}
	}
	return out, nil
}
