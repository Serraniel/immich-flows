package tags

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestBulkMoves(t *testing.T) {
	values := []string{"XX", "XX/child", "XY", "other", "service-a/cat", "service-a/dog"}
	cases := []struct {
		b    Bulk
		want string
		err  bool
	}{
		{Bulk{Match: `^X.$`, Replace: "YY/$0"}, "[{XX YY/XX} {XY YY/XY}]", false},
		{Bulk{Match: `^service-a/(.*)$`, Replace: "services/a/$1"}, "[{service-a/cat services/a/cat} {service-a/dog services/a/dog}]", false},
		{Bulk{Match: `^XX$`, Replace: "XX"}, "[]", false},
		{Bulk{Match: `^X.$`, Replace: "same"}, "", true},
		{Bulk{Match: `^(XX|other)$`, Replace: "XX/${1}x"}, "", true},
		{Bulk{Match: `(`, Replace: "x"}, "", true},
	}
	for _, c := range cases {
		moves, err := c.b.moves(values)
		if c.err {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("%+v: want ErrInvalid, got %v", c.b, err)
			}
			continue
		}
		got := fmt.Sprint(moves)
		if len(moves) == 0 {
			got = "[]"
		}
		if err != nil || got != c.want {
			t.Errorf("%+v: %v %v", c.b, moves, err)
		}
	}
}

func TestApplyBulk(t *testing.T) {
	s, f := setup(t)
	f.Tag(asset(1), "service-a/cat")
	f.Tag(asset(2), "service-a/dog/young")
	f.Tag(asset(3), "keep")
	if _, err := s.PlanBulk(context.Background(), Bulk{Match: `^service-a/`, Replace: "services/a/"}); err != nil {
		t.Fatal(err)
	}
	if assets, _ := f.State(); assets != asset(1)+": service-a/cat; "+asset(2)+": service-a/dog/young; "+asset(3)+": keep" {
		t.Fatalf("plan changed state: %s", assets)
	}
	plan, err := s.ApplyBulk(context.Background(), Bulk{Match: `^service-a/`, Replace: "services/a/"})
	if err != nil {
		t.Fatal(err)
	}
	assets, _ := f.State()
	if len(plan.Moves) != 2 || assets != asset(1)+": services/a/cat; "+asset(2)+": services/a/dog/young; "+asset(3)+": keep" {
		t.Errorf("%d moves, %s", len(plan.Moves), assets)
	}
}
