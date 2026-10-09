package hooks

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/proxy"
)

type duplicatesKey struct{}

type duplicateGroup struct {
	DuplicateID   string   `json:"duplicateId"`
	KeepAssetIDs  []string `json:"keepAssetIds"`
	TrashAssetIDs []string `json:"trashAssetIds"`
}

type duplicatesState struct {
	groups    []duplicateGroup
	snapshots map[string]any
}

// Duplicates publishes duplicates.resolved for each resolved group, with a snapshot of all assets
// in the group (including tags) taken before Immich merged and trashed them.
func Duplicates(d Deps) proxy.Hook {
	return proxy.Hook{
		Route:   "POST /api/duplicates/resolve",
		Timeout: 30 * time.Second,
		Pre: func(ctx context.Context, ex *proxy.Exchange) error {
			if !d.wants(events.DuplicatesResolved) && !d.wants(events.AssetTrashed) {
				return nil
			}
			var body struct {
				Groups []duplicateGroup `json:"groups"`
			}
			if err := readJSON(ex.Request, &body); err != nil {
				return err
			}
			st := &duplicatesState{groups: body.Groups}
			if d.wants(events.DuplicatesResolved) {
				var ids []string
				for _, g := range body.Groups {
					ids = append(append(ids, g.KeepAssetIDs...), g.TrashAssetIDs...)
				}
				c, err := d.client(ex.Request)
				if err != nil {
					return err
				}
				st.snapshots = snapshot(ctx, c, ids)
			}
			ex.Set(duplicatesKey{}, st)
			return nil
		},
		Post: func(ex *proxy.Exchange) {
			st, _ := ex.Get(duplicatesKey{}).(*duplicatesState)
			if st == nil || ex.Status != http.StatusOK {
				return
			}
			ok := resolvedGroups(ex.Response)
			now := time.Now().UTC()
			var trashed []string
			for _, g := range st.groups {
				if !ok[g.DuplicateID] {
					continue
				}
				trashed = append(trashed, g.TrashAssetIDs...)
				if !d.wants(events.DuplicatesResolved) || len(g.KeepAssetIDs) == 0 {
					continue
				}
				group := map[string]any{}
				for _, id := range append(append([]string{}, g.KeepAssetIDs...), g.TrashAssetIDs...) {
					if s, ok := st.snapshots[id]; ok {
						group[id] = s
					}
				}
				d.publish(ex.Request, events.Event{
					Type:     events.DuplicatesResolved,
					Time:     now,
					AssetIDs: g.KeepAssetIDs,
					Data: map[string]any{
						"duplicateId": g.DuplicateID,
						"kept":        g.KeepAssetIDs,
						"trashed":     g.TrashAssetIDs,
						"assets":      group,
						"tags":        tagsByAsset(group),
					},
				})
			}
			if len(trashed) > 0 && d.wants(events.AssetTrashed) {
				d.publish(ex.Request, events.Event{Type: events.AssetTrashed, Time: now, AssetIDs: trashed})
			}
		},
	}
}

// resolvedGroups returns the duplicate IDs Immich reports as resolved.
func resolvedGroups(resp []byte) map[string]bool {
	var items []struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
	}
	out := map[string]bool{}
	if json.Unmarshal(resp, &items) != nil {
		return out
	}
	for _, it := range items {
		if it.Success {
			out[it.ID] = true
		}
	}
	return out
}

// tagsByAsset maps asset ID to its tag values, from snapshots.
func tagsByAsset(snapshots map[string]any) map[string]any {
	out := map[string]any{}
	for id, s := range snapshots {
		m, _ := s.(map[string]any)
		tags, _ := m["tags"].([]any)
		values := []any{}
		for _, t := range tags {
			if tm, ok := t.(map[string]any); ok {
				values = append(values, tm["value"])
			}
		}
		out[id] = values
	}
	return out
}
