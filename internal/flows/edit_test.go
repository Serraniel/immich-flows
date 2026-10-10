package flows

import (
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/flow"
)

func TestCheckSource(t *testing.T) {
	for _, s := range []string{"a.yaml", "sub/b.YML"} {
		if err := checkSource(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"", "a.txt", "../a.yaml", "/a.yaml", ".a.yaml", ".git/a.yaml", "a/../b.yaml", "a//b.yaml"} {
		if err := checkSource(s); !errors.Is(err, ErrSource) {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "flows") // created on first write
	d := NewDir(dir, engine.NewRegistry(), slog.New(slog.DiscardHandler))
	if err := d.Load(); err != nil {
		t.Fatal(err)
	}

	if err := d.Write("sub/a.yaml", []byte(flowYAML("a"))); err != nil {
		t.Fatal(err)
	}
	if got := ids(d); len(got) != 1 || got[0] != "a" {
		t.Fatalf("flows: %v", got)
	}
	data, err := d.Read("sub/a.yaml")
	if err != nil || string(data) != flowYAML("a") {
		t.Fatalf("read: %q %v", data, err)
	}

	var verrs flow.Errors
	if err := d.Write("b.yaml", []byte(flowYAML("a"))); !errors.As(err, &verrs) {
		t.Errorf("duplicate id: %v", err)
	}
	if err := d.Write("b.yaml", []byte("id: b\ntrigger: {on: manual}\nsteps: [{use: nope}]\n")); !errors.As(err, &verrs) || verrs[0].Line != 3 {
		t.Errorf("unknown step: %v", err)
	}
	if _, err := d.Read("b.yaml"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("invalid content written: %v", err)
	}
	// Rewriting the same file with its own ID is fine.
	if err := d.Write("sub/a.yaml", []byte(flowYAML("a"))); err != nil {
		t.Error(err)
	}

	if err := d.SetEnabled("sub/a.yaml", false); err != nil {
		t.Fatal(err)
	}
	if f, _ := d.Flow("a"); f == nil || f.IsEnabled() {
		t.Errorf("not disabled: %+v", f)
	}

	if err := d.Delete("sub/a.yaml"); err != nil {
		t.Fatal(err)
	}
	if len(d.Entries()) != 0 {
		t.Errorf("entries after delete: %+v", d.Entries())
	}
	if err := d.Delete("../x.yaml"); !errors.Is(err, ErrSource) {
		t.Errorf("escape: %v", err)
	}
}
