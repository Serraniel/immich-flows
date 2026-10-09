package tags

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/serraniel/immich-flows/internal/immich"
)

// Bulk moves every tag whose value matches Match to Replace (regexp.Expand syntax: $1, ${name}).
// Children of a matching tag move with it and are not matched separately.
type Bulk struct {
	Match   string `json:"match"`
	Replace string `json:"replace"`
}

// BulkPlan lists the moves of a bulk operation.
type BulkPlan struct {
	Moves []*Plan `json:"moves"`
}

func (b Bulk) moves(values []string) ([]Move, error) {
	re, err := regexp.Compile(b.Match)
	if err != nil {
		return nil, fmt.Errorf("%w: match: %w", ErrInvalid, err)
	}
	sort.Strings(values)
	var out []Move
	targets := map[string]string{}
	for _, v := range values {
		if !re.MatchString(v) || covered(out, v) {
			continue
		}
		to := normalize(re.ReplaceAllString(v, b.Replace))
		if to == "" || to == v {
			continue
		}
		if prev, dup := targets[to]; dup {
			return nil, fmt.Errorf("%w: %q and %q both map to %q", ErrInvalid, prev, v, to)
		}
		targets[to] = v
		out = append(out, Move{From: v, To: to})
	}
	for _, m := range out {
		for _, other := range out {
			if m != other && (m.To == other.From || strings.HasPrefix(m.To, other.From+"/")) {
				return nil, fmt.Errorf("%w: target %q is inside source %q; run in two steps", ErrInvalid, m.To, other.From)
			}
		}
	}
	return out, nil
}

// covered reports whether v is a descendant of an earlier move's source.
func covered(moves []Move, v string) bool {
	for _, m := range moves {
		if strings.HasPrefix(v, m.From+"/") {
			return true
		}
	}
	return false
}

// PlanBulk computes all moves without changing anything.
func (s *Service) PlanBulk(ctx context.Context, b Bulk) (*BulkPlan, error) {
	return s.bulk(ctx, b, s.Plan)
}

// ApplyBulk performs all moves in order; it stops at the first error.
func (s *Service) ApplyBulk(ctx context.Context, b Bulk) (*BulkPlan, error) {
	return s.bulk(ctx, b, s.Apply)
}

func (s *Service) bulk(ctx context.Context, b Bulk, op func(context.Context, Move) (*Plan, error)) (*BulkPlan, error) {
	res, err := s.c.GetAllTagsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err := immich.Check(res, res.Body); err != nil {
		return nil, err
	}
	values := make([]string, 0, len(*res.JSON200))
	for _, t := range *res.JSON200 {
		values = append(values, t.Value)
	}
	moves, err := b.moves(values)
	if err != nil {
		return nil, err
	}
	out := &BulkPlan{Moves: []*Plan{}}
	for _, m := range moves {
		p, err := op(ctx, m)
		if err != nil {
			return out, fmt.Errorf("move %q to %q: %w", m.From, m.To, err)
		}
		out.Moves = append(out.Moves, p)
	}
	return out, nil
}
