package tags

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/immichtest"
)

func asset(i int) string { return fmt.Sprintf("10000000-0000-4000-8000-%012d", i) }

func setup(t *testing.T) (*Service, *immichtest.Fake) {
	t.Helper()
	f := immichtest.New()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	c, err := immich.New(u, immich.APIKey("k"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return New(c), f
}

func TestMoveWithChildren(t *testing.T) {
	s, f := setup(t)
	f.Tag(asset(1), "XX")
	f.Tag(asset(2), "XX/child")
	f.Tag(asset(3), "other")
	f.ByValue("XX").Color = "#ff0000"

	plan, err := s.Plan(context.Background(), Move{From: "XX", To: "YY/XX"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Assets != 2 || len(plan.Changes) != 2 || plan.Changes[1].To != "YY/XX/child" || plan.Changes[1].Assets != 1 {
		t.Errorf("plan %+v", plan)
	}
	if assets, _ := f.State(); assets != asset(1)+": XX; "+asset(2)+": XX/child; "+asset(3)+": other" {
		t.Errorf("plan changed state: %s", assets)
	}

	if _, err := s.Apply(context.Background(), Move{From: "XX", To: "YY/XX"}); err != nil {
		t.Fatal(err)
	}
	assets, values := f.State()
	if assets != asset(1)+": YY/XX; "+asset(2)+": YY/XX/child; "+asset(3)+": other" {
		t.Errorf("assets: %s", assets)
	}
	if values != "YY,YY/XX,YY/XX/child,other" {
		t.Errorf("tags: %s", values)
	}
	if f.ByValue("YY/XX").Color != "#ff0000" {
		t.Error("color not kept")
	}
}

func TestMergeIntoExisting(t *testing.T) {
	s, f := setup(t)
	f.Tag(asset(1), "cats")
	f.Tag(asset(2), "cat")
	f.Tag(asset(3), "cat")
	f.Tag(asset(3), "cats")

	plan, err := s.Apply(context.Background(), Move{From: "cat", To: "cats"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changes[0].Exists {
		t.Error("merge not detected")
	}
	assets, values := f.State()
	if assets != asset(1)+": cats; "+asset(2)+": cats; "+asset(3)+": cats" || values != "cats" {
		t.Errorf("%s | %s", assets, values)
	}
}

func TestRenameKeepsParent(t *testing.T) {
	s, f := setup(t)
	f.Tag(asset(1), "a/b")
	f.Tag(asset(2), "a")
	if _, err := s.Apply(context.Background(), Move{From: "a/b", To: "a/c"}); err != nil {
		t.Fatal(err)
	}
	if assets, values := f.State(); assets != asset(1)+": a/c; "+asset(2)+": a" || values != "a,a/c" {
		t.Errorf("%s | %s", assets, values)
	}
}

func TestPagination(t *testing.T) {
	s, f := setup(t)
	for i := range pageSize + 5 {
		f.Tag(asset(i), "bulk")
	}
	plan, err := s.Plan(context.Background(), Move{From: "bulk", To: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Assets != pageSize+5 || f.Pages != 2 {
		t.Errorf("assets=%d pages=%d", plan.Assets, f.Pages)
	}
}

func TestInvalid(t *testing.T) {
	s, f := setup(t)
	f.Tag(asset(1), "a")
	for _, m := range []Move{{"a", "a"}, {"a", "a/b"}, {"", "x"}, {"missing", "x"}, {" a / ", "a"}} {
		if _, err := s.Plan(context.Background(), m); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", m, err)
		}
	}
}

func TestApplySettlesAfterResurrection(t *testing.T) {
	fallbackWait = time.Millisecond
	for _, forbidden := range []bool{false, true} {
		s, f := setup(t)
		f.Tag(asset(1), "old")
		f.Resurrect, f.QueuesForbidden = true, forbidden
		plan, err := s.Apply(context.Background(), Move{From: "old", To: "new"})
		if err != nil {
			t.Fatal(err)
		}
		assets, values := f.State()
		if !plan.Settled || f.Deletes != 2 || assets != asset(1)+": new" || values != "new" {
			t.Errorf("forbidden=%v settled=%v deletes=%d %s | %s", forbidden, plan.Settled, f.Deletes, assets, values)
		}
	}
}
