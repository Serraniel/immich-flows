package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"golang.org/x/sync/errgroup"

	"github.com/serraniel/immich-flows/internal/immich"
)

const (
	maxSnapshots  = 500
	maxBody       = 1 << 20
	listPageSize  = 1000
	maxTrashList  = 100000
	snapshotLimit = 8
)

// readJSON decodes a small JSON request body and restores it for the upstream.
func readJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return io.EOF
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// snapshot returns asset info (as exposed to flows) for up to maxSnapshots assets.
// Missing or inaccessible assets are left out.
func snapshot(ctx context.Context, c *immich.ClientWithResponses, ids []string) map[string]any {
	if len(ids) > maxSnapshots {
		ids = ids[:maxSnapshots]
	}
	out := map[string]any{}
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(snapshotLimit)
	for _, id := range ids {
		g.Go(func() error {
			var uid openapi_types.UUID
			if uid.UnmarshalText([]byte(id)) != nil {
				return nil
			}
			res, err := c.GetAssetInfoWithResponse(ctx, uid, nil)
			if err != nil || res.StatusCode() != http.StatusOK {
				return nil
			}
			var m map[string]any
			if json.Unmarshal(res.Body, &m) == nil {
				mu.Lock()
				out[id] = m
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	return out
}

// trashedAssets lists the caller's trashed asset IDs.
func trashedAssets(ctx context.Context, c *immich.ClientWithResponses) ([]string, error) {
	var ids []string
	withDeleted, size := true, listPageSize
	epoch := time.Unix(0, 0).UTC()
	for page := 1; len(ids) < maxTrashList; page++ {
		p := page
		res, err := c.SearchAssetsWithResponse(ctx, nil, immich.MetadataSearchDto{
			WithDeleted: &withDeleted, TrashedAfter: &epoch, Page: &p, Size: &size,
		})
		if err != nil {
			return nil, err
		}
		if err := immich.Check(res, res.Body); err != nil {
			return nil, err
		}
		for _, a := range res.JSON200.Assets.Items {
			ids = append(ids, a.Id.String())
		}
		if res.JSON200.Assets.NextPage == nil {
			break
		}
	}
	return ids, nil
}
