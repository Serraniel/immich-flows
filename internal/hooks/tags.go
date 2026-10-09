package hooks

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/proxy"
)

type tagChange struct {
	typ    string
	tags   []map[string]any // {id, value}
	assets []string
}

type tagKey struct{}

// Tags publishes tag.assigned and tag.removed for tag changes made by users through the proxy.
// Changes made by flows go directly to Immich and do not trigger these events.
func Tags(d Deps) []proxy.Hook {
	single := func(route, typ string) proxy.Hook {
		return proxy.Hook{
			Route: route,
			Pre: func(ctx context.Context, ex *proxy.Exchange) error {
				if !d.wants(typ) {
					return nil
				}
				var body struct {
					IDs []string `json:"ids"`
				}
				if err := readJSON(ex.Request, &body); err != nil {
					return err
				}
				tags, err := d.lookupTags(ctx, ex.Request, []string{ex.Request.PathValue("id")})
				if err != nil {
					return err
				}
				ex.Set(tagKey{}, &tagChange{typ: typ, tags: tags, assets: body.IDs})
				return nil
			},
			Post: func(ex *proxy.Exchange) {
				ch, _ := ex.Get(tagKey{}).(*tagChange)
				if ch == nil || ex.Status != http.StatusOK {
					return
				}
				// Only assets Immich reports as changed.
				var results []struct {
					ID      string `json:"id"`
					Success bool   `json:"success"`
				}
				_ = json.Unmarshal(ex.Response, &results)
				var changed []string
				for _, r := range results {
					if r.Success {
						changed = append(changed, r.ID)
					}
				}
				ch.assets = changed
				d.publishTags(ex.Request, ch)
			},
		}
	}

	bulk := proxy.Hook{
		Route: "PUT /api/tags/assets",
		Pre: func(ctx context.Context, ex *proxy.Exchange) error {
			if !d.wants(events.TagAssigned) {
				return nil
			}
			var body struct {
				TagIDs   []string `json:"tagIds"`
				AssetIDs []string `json:"assetIds"`
			}
			if err := readJSON(ex.Request, &body); err != nil {
				return err
			}
			tags, err := d.lookupTags(ctx, ex.Request, body.TagIDs)
			if err != nil {
				return err
			}
			ex.Set(tagKey{}, &tagChange{typ: events.TagAssigned, tags: tags, assets: body.AssetIDs})
			return nil
		},
		Post: func(ex *proxy.Exchange) {
			if ch, _ := ex.Get(tagKey{}).(*tagChange); ch != nil && ex.Status == http.StatusOK {
				d.publishTags(ex.Request, ch)
			}
		},
	}

	return []proxy.Hook{
		single("PUT /api/tags/{id}/assets", events.TagAssigned),
		single("DELETE /api/tags/{id}/assets", events.TagRemoved),
		bulk,
	}
}

// publishTags emits one event per tag; flows run once per asset.
func (d Deps) publishTags(r *http.Request, ch *tagChange) {
	if len(ch.assets) == 0 {
		return
	}
	now := time.Now().UTC()
	for _, t := range ch.tags {
		d.publish(r, events.Event{Type: ch.typ, Time: now, AssetIDs: ch.assets, Data: map[string]any{"tag": t}})
	}
}

// lookupTags resolves tag IDs to {id, value} with the caller's credentials. Unknown IDs are skipped.
func (d Deps) lookupTags(ctx context.Context, r *http.Request, ids []string) ([]map[string]any, error) {
	c, err := d.client(r)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, id := range ids {
		var uid openapi_types.UUID
		if uid.UnmarshalText([]byte(id)) != nil {
			continue
		}
		res, err := c.GetTagByIdWithResponse(ctx, uid)
		if err != nil {
			return nil, err
		}
		if immich.Check(res, res.Body) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "value": res.JSON200.Value, "name": res.JSON200.Name})
	}
	return out, nil
}
