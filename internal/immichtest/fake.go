// Package immichtest provides an in-memory fake of the Immich tag, asset and search API for tests.
package immichtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Fake implements the tag, asset and search endpoints with Immich's semantics:
// search by tag includes descendants, deleting a tag deletes its descendants.
type Fake struct {
	mu     sync.Mutex
	n      int
	tags   map[string]*Tag // id -> tag
	assets map[string]map[string]bool
	// Pages counts search requests.
	Pages int
	// Resurrect re-adds a deleted tag to its assets once, like Immich's sidecar re-import race.
	Resurrect bool
	// QueuesForbidden makes GET /queues return 403 (non-admin caller).
	QueuesForbidden bool
	// Deletes counts tag deletions.
	Deletes int
	// Requests counts requests by "METHOD /path".
	Requests map[string]int
	// Originals holds file contents served by GET /assets/{id}/original.
	Originals map[string][]byte
	// Info holds extra asset fields returned by GET /assets/{id} (e.g. checksum, exifInfo).
	Info map[string]map[string]any
	// Metadata holds asset metadata written via PUT /assets/{id}/metadata.
	Metadata map[string]map[string]any
}

// Tag is a fake tag.
type Tag struct {
	ID, Value, Color string
}

// New returns an empty Fake.
func New() *Fake {
	return &Fake{tags: map[string]*Tag{}, assets: map[string]map[string]bool{}, Requests: map[string]int{},
		Originals: map[string][]byte{}, Info: map[string]map[string]any{}, Metadata: map[string]map[string]any{}}
}

func (f *Fake) id() string {
	f.n++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", f.n)
}

// ByValue returns the tag with value v.
func (f *Fake) ByValue(v string) *Tag {
	for _, t := range f.tags {
		if t.Value == v {
			return t
		}
	}
	return nil
}

func (f *Fake) upsert(v string) *Tag {
	parts := strings.Split(v, "/")
	var t *Tag
	for i := range parts {
		p := strings.Join(parts[:i+1], "/")
		if t = f.ByValue(p); t == nil {
			t = &Tag{ID: f.id(), Value: p}
			f.tags[t.ID] = t
		}
	}
	return t
}

// Tag creates the tag (with parents) and assigns it to the asset.
func (f *Fake) Tag(asset, value string) {
	t := f.upsert(value)
	if f.assets[asset] == nil {
		f.assets[asset] = map[string]bool{}
	}
	f.assets[asset][t.ID] = true
}

// State returns "asset: tags" lines and all tag values, sorted.
func (f *Fake) State() (string, string) {
	var lines, values []string
	for a, ts := range f.assets {
		var vs []string
		for id := range ts {
			vs = append(vs, f.tags[id].Value)
		}
		sort.Strings(vs)
		lines = append(lines, a+": "+strings.Join(vs, ","))
	}
	for _, t := range f.tags {
		values = append(values, t.Value)
	}
	sort.Strings(lines)
	sort.Strings(values)
	return strings.Join(lines, "; "), strings.Join(values, ",")
}

func (f *Fake) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *Fake) tagDTO(t *Tag) map[string]any {
	parts := strings.Split(t.Value, "/")
	m := map[string]any{"id": t.ID, "value": t.Value, "name": parts[len(parts)-1], "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z"}
	if t.Color != "" {
		m["color"] = t.Color
	}
	return m
}

// ServeHTTP implements http.Handler.
func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api")
	f.Requests[r.Method+" "+path]++
	switch {
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/tags/") && strings.HasSuffix(path, "/assets"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/tags/"), "/assets")
		var body struct {
			IDs []string `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		out := []any{}
		for _, a := range body.IDs {
			delete(f.assets[a], id)
			out = append(out, map[string]any{"id": a, "success": true})
		}
		f.json(w, out)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/assets/") && strings.HasSuffix(path, "/original"):
		data, ok := f.Originals[strings.TrimSuffix(strings.TrimPrefix(path, "/assets/"), "/original")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	case r.Method == http.MethodPut && strings.HasPrefix(path, "/assets/") && strings.HasSuffix(path, "/metadata"):
		a := strings.TrimSuffix(strings.TrimPrefix(path, "/assets/"), "/metadata")
		var body struct {
			Items []struct {
				Key   string         `json:"key"`
				Value map[string]any `json:"value"`
			} `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if f.Metadata[a] == nil {
			f.Metadata[a] = map[string]any{}
		}
		out := []any{}
		for _, it := range body.Items {
			f.Metadata[a][it.Key] = it.Value
			out = append(out, map[string]any{"key": it.Key, "value": it.Value, "updatedAt": "2026-01-01T00:00:00Z"})
		}
		f.json(w, out)
	case r.Method == http.MethodGet && path == "/tags":
		out := []any{}
		for _, t := range f.tags {
			out = append(out, f.tagDTO(t))
		}
		f.json(w, out)
	case r.Method == http.MethodPut && path == "/tags":
		var body struct{ Tags []string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		out := []any{}
		for _, v := range body.Tags {
			out = append(out, f.tagDTO(f.upsert(v)))
		}
		f.json(w, out)
	case r.Method == http.MethodPut && path == "/tags/assets":
		var body struct {
			TagIDs   []string `json:"tagIds"`
			AssetIDs []string `json:"assetIds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, a := range body.AssetIDs {
			for _, t := range body.TagIDs {
				if f.assets[a] == nil {
					f.assets[a] = map[string]bool{}
				}
				f.assets[a][t] = true
			}
		}
		f.json(w, map[string]int{"count": len(body.AssetIDs)})
	case r.Method == http.MethodPut && strings.HasPrefix(path, "/tags/"):
		var body struct{ Color string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		t := f.tags[strings.TrimPrefix(path, "/tags/")]
		t.Color = body.Color
		f.json(w, f.tagDTO(t))
	case r.Method == http.MethodGet && path == "/queues":
		if f.QueuesForbidden {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		f.json(w, []any{map[string]any{"name": "sidecar", "isPaused": false, "statistics": map[string]int{}}})
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/tags/"):
		f.Deletes++
		root := f.tags[strings.TrimPrefix(path, "/tags/")]
		var had []string
		for a, ts := range f.assets {
			if ts[root.ID] {
				had = append(had, a)
			}
		}
		defer func() {
			if f.Resurrect {
				f.Resurrect = false
				for _, a := range had {
					f.Tag(a, root.Value)
				}
			}
		}()
		for id, t := range f.tags {
			if t.Value == root.Value || strings.HasPrefix(t.Value, root.Value+"/") {
				delete(f.tags, id)
				for _, ts := range f.assets {
					delete(ts, id)
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && path == "/search/metadata":
		var body struct {
			TagIDs     []string `json:"tagIds"`
			Page, Size int
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.Pages++
		root := f.tags[body.TagIDs[0]]
		var ids []string
		for a, ts := range f.assets {
			for id := range ts {
				v := f.tags[id].Value
				if v == root.Value || strings.HasPrefix(v, root.Value+"/") {
					ids = append(ids, a)
					break
				}
			}
		}
		sort.Strings(ids)
		start := (body.Page - 1) * body.Size
		end := min(start+body.Size, len(ids))
		items := []any{}
		for _, id := range ids[min(start, len(ids)):end] {
			items = append(items, map[string]any{"id": id})
		}
		var next any
		if end < len(ids) {
			next = fmt.Sprint(body.Page + 1)
		}
		f.json(w, map[string]any{"assets": map[string]any{"items": items, "nextPage": next, "total": len(ids), "count": len(items), "facets": []any{}},
			"albums": map[string]any{"items": []any{}, "total": 0, "count": 0, "facets": []any{}}})
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/assets/"):
		a := strings.TrimPrefix(path, "/assets/")
		tags := []any{}
		for id := range f.assets[a] {
			tags = append(tags, f.tagDTO(f.tags[id]))
		}
		m := map[string]any{"id": a, "tags": tags}
		for k, v := range f.Info[a] {
			m[k] = v
		}
		f.json(w, m)
	default:
		http.Error(w, "not implemented: "+r.Method+" "+path, http.StatusNotImplemented)
	}
}
