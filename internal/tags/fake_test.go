package tags

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// fakeImmich implements the tag and search endpoints with Immich's semantics:
// search by tag includes descendants, deleting a tag deletes its descendants.
type fakeImmich struct {
	mu     sync.Mutex
	n      int
	tags   map[string]*fakeTag // id -> tag
	assets map[string]map[string]bool
	pages  int
}

type fakeTag struct {
	ID, Value, Color string
}

func newFake() *fakeImmich {
	return &fakeImmich{tags: map[string]*fakeTag{}, assets: map[string]map[string]bool{}}
}

func (f *fakeImmich) id() string {
	f.n++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", f.n)
}

func (f *fakeImmich) byValue(v string) *fakeTag {
	for _, t := range f.tags {
		if t.Value == v {
			return t
		}
	}
	return nil
}

func (f *fakeImmich) upsert(v string) *fakeTag {
	parts := strings.Split(v, "/")
	var t *fakeTag
	for i := range parts {
		p := strings.Join(parts[:i+1], "/")
		if t = f.byValue(p); t == nil {
			t = &fakeTag{ID: f.id(), Value: p}
			f.tags[t.ID] = t
		}
	}
	return t
}

// tag creates the tag (with parents) and assigns it to the asset.
func (f *fakeImmich) tag(asset, value string) {
	t := f.upsert(value)
	if f.assets[asset] == nil {
		f.assets[asset] = map[string]bool{}
	}
	f.assets[asset][t.ID] = true
}

// state returns "asset: tags" lines and all tag values, sorted.
func (f *fakeImmich) state() (string, string) {
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

func (f *fakeImmich) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeImmich) tagDTO(t *fakeTag) map[string]any {
	parts := strings.Split(t.Value, "/")
	m := map[string]any{"id": t.ID, "value": t.Value, "name": parts[len(parts)-1], "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z"}
	if t.Color != "" {
		m["color"] = t.Color
	}
	return m
}

func (f *fakeImmich) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api")
	switch {
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
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/tags/"):
		root := f.tags[strings.TrimPrefix(path, "/tags/")]
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
		f.pages++
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
		f.json(w, map[string]any{"id": a, "tags": tags})
	default:
		http.Error(w, "not implemented: "+r.Method+" "+path, http.StatusNotImplemented)
	}
}
