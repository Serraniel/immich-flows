// Package tags implements bulk tag operations Immich lacks: move, rename and merge with children.
package tags

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"golang.org/x/sync/errgroup"

	"github.com/serraniel/immich-flows/internal/immich"
)

const (
	pageSize    = 1000
	concurrency = 8
	tagBatch    = 500
)

// ErrInvalid is returned for impossible moves (same path, into own subtree, unknown source).
var ErrInvalid = errors.New("invalid tag move")

// Move describes moving the tag at From (and its children) to To. If To exists, tags are merged.
// Renaming is a move within the same parent.
type Move struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Change is one tag of the source subtree and where it goes.
type Change struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Exists bool   `json:"exists"` // target already exists (merge)
	Assets int    `json:"assets"` // assets tagged with exactly this tag
}

// Plan is the result of a dry run.
type Plan struct {
	Move    Move     `json:"move"`
	Changes []Change `json:"changes"`
	Assets  int      `json:"assets"` // distinct assets affected
}

// Service runs tag operations with one user's Immich client.
type Service struct {
	c *immich.ClientWithResponses
}

// New returns a Service acting with c.
func New(c *immich.ClientWithResponses) *Service { return &Service{c: c} }

func normalize(v string) string {
	parts := strings.Split(v, "/")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "/")
}

type state struct {
	byValue map[string]immich.TagResponseDto
	source  immich.TagResponseDto
	// assetTargets maps target value -> asset IDs that get it.
	assetTargets map[string][]openapi_types.UUID
	changes      []Change
	assets       int
}

// Plan computes a move without changing anything.
func (s *Service) Plan(ctx context.Context, m Move) (*Plan, error) {
	st, err := s.prepare(ctx, m)
	if err != nil {
		return nil, err
	}
	return &Plan{Move: Move{From: normalize(m.From), To: normalize(m.To)}, Changes: st.changes, Assets: st.assets}, nil
}

// Apply performs a move: create targets, tag assets, delete the source subtree.
// It is safe to re-run after a partial failure.
func (s *Service) Apply(ctx context.Context, m Move) (*Plan, error) {
	st, err := s.prepare(ctx, m)
	if err != nil {
		return nil, err
	}
	targets := make([]string, 0, len(st.changes))
	for _, c := range st.changes {
		targets = append(targets, c.To)
	}
	up, err := s.c.UpsertTagsWithResponse(ctx, immich.TagUpsertDto{Tags: targets})
	if err != nil {
		return nil, err
	}
	if err := immich.Check(up, up.Body); err != nil {
		return nil, err
	}
	ids := map[string]openapi_types.UUID{}
	for _, t := range *up.JSON200 {
		ids[t.Value] = t.Id
	}

	// Keep the source color on a newly created root target.
	root := st.changes[0]
	if !root.Exists && st.source.Color != nil {
		if id, ok := ids[root.To]; ok {
			res, err := s.c.UpdateTagWithResponse(ctx, id, immich.TagUpdateDto{Color: st.source.Color})
			if err == nil {
				_ = immich.Check(res, res.Body)
			}
		}
	}

	for value, assets := range st.assetTargets {
		id, ok := ids[value]
		if !ok {
			return nil, fmt.Errorf("tag %q missing after upsert", value)
		}
		for i := 0; i < len(assets); i += tagBatch {
			batch := assets[i:min(i+tagBatch, len(assets))]
			res, err := s.c.BulkTagAssetsWithResponse(ctx, immich.TagBulkAssetsDto{TagIds: []openapi_types.UUID{id}, AssetIds: batch})
			if err != nil {
				return nil, err
			}
			if err := immich.Check(res, res.Body); err != nil {
				return nil, err
			}
		}
	}

	del, err := s.c.DeleteTagWithResponse(ctx, st.source.Id)
	if err != nil {
		return nil, err
	}
	if err := immich.Check(del, del.Body); err != nil {
		return nil, err
	}
	return &Plan{Move: Move{From: normalize(m.From), To: normalize(m.To)}, Changes: st.changes, Assets: st.assets}, nil
}

func (s *Service) prepare(ctx context.Context, m Move) (*state, error) {
	from, to := normalize(m.From), normalize(m.To)
	switch {
	case from == "" || to == "":
		return nil, fmt.Errorf("%w: from and to are required", ErrInvalid)
	case from == to:
		return nil, fmt.Errorf("%w: from and to are equal", ErrInvalid)
	case strings.HasPrefix(to, from+"/"):
		return nil, fmt.Errorf("%w: cannot move %q into itself", ErrInvalid, from)
	}

	res, err := s.c.GetAllTagsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err := immich.Check(res, res.Body); err != nil {
		return nil, err
	}
	st := &state{byValue: map[string]immich.TagResponseDto{}, assetTargets: map[string][]openapi_types.UUID{}}
	for _, t := range *res.JSON200 {
		st.byValue[t.Value] = t
	}
	src, ok := st.byValue[from]
	if !ok {
		return nil, fmt.Errorf("%w: tag %q not found", ErrInvalid, from)
	}
	st.source = src

	retarget := func(v string) (string, bool) {
		if v == from {
			return to, true
		}
		if strings.HasPrefix(v, from+"/") {
			return to + v[len(from):], true
		}
		return "", false
	}

	counts := map[string]int{}
	for v := range st.byValue {
		if _, ok := retarget(v); ok {
			counts[v] = 0
		}
	}

	assets, err := s.assetsUnder(ctx, src.Id)
	if err != nil {
		return nil, err
	}
	st.assets = len(assets)
	tagsOf, err := s.assetTags(ctx, assets)
	if err != nil {
		return nil, err
	}
	for i, id := range assets {
		for _, v := range tagsOf[i] {
			if target, ok := retarget(v); ok {
				st.assetTargets[target] = append(st.assetTargets[target], id)
				counts[v]++
			}
		}
	}

	for v, n := range counts {
		target, _ := retarget(v)
		_, exists := st.byValue[target]
		st.changes = append(st.changes, Change{From: v, To: target, Exists: exists, Assets: n})
	}
	sort.Slice(st.changes, func(i, j int) bool { return st.changes[i].From < st.changes[j].From })
	return st, nil
}

// assetsUnder returns all assets tagged with the tag or a descendant, including trashed ones.
func (s *Service) assetsUnder(ctx context.Context, tagID openapi_types.UUID) ([]openapi_types.UUID, error) {
	var out []openapi_types.UUID
	withDeleted, size := true, pageSize
	for page := 1; ; page++ {
		p := page
		res, err := s.c.SearchAssetsWithResponse(ctx, nil, immich.MetadataSearchDto{
			TagIds: &[]openapi_types.UUID{tagID}, WithDeleted: &withDeleted, Page: &p, Size: &size,
		})
		if err != nil {
			return nil, err
		}
		if err := immich.Check(res, res.Body); err != nil {
			return nil, err
		}
		for _, a := range res.JSON200.Assets.Items {
			out = append(out, a.Id)
		}
		if res.JSON200.Assets.NextPage == nil {
			return out, nil
		}
	}
}

// assetTags returns the tag values each asset is directly tagged with.
func (s *Service) assetTags(ctx context.Context, assets []openapi_types.UUID) ([][]string, error) {
	out := make([][]string, len(assets))
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)
	for i, id := range assets {
		g.Go(func() error {
			res, err := s.c.GetAssetInfoWithResponse(ctx, id, nil)
			if err != nil {
				return err
			}
			if err := immich.Check(res, res.Body); err != nil {
				return err
			}
			var values []string
			if res.JSON200.Tags != nil {
				for _, t := range *res.JSON200.Tags {
					values = append(values, t.Value)
				}
			}
			mu.Lock()
			out[i] = values
			mu.Unlock()
			return nil
		})
	}
	return out, g.Wait()
}
