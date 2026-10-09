package flows

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serraniel/immich-flows/internal/engine"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const valid = "id: %s\ntrigger: {on: manual}\nsteps: [{use: log, with: {message: hi}}]\n"

func flowYAML(id string) string { return strings.Replace(valid, "%s", id, 1) }

func ids(d *Dir) []string {
	var out []string
	for _, f := range d.Flows() {
		out = append(out, f.ID)
	}
	return out
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.yaml", flowYAML("a"))
	write(t, dir, "sub/b.yml", flowYAML("b"))
	write(t, dir, "dup.yaml", flowYAML("a"))
	write(t, dir, "broken.yaml", "id: [")
	write(t, dir, "unknown-step.yaml", "id: c\ntrigger: {on: manual}\nsteps: [{use: nope}]\n")
	write(t, dir, "notes.txt", "ignored")
	write(t, dir, ".hidden.yaml", flowYAML("h"))
	write(t, dir, ".git/x.yaml", flowYAML("g"))

	d := NewDir(dir, engine.NewRegistry(), slog.New(slog.DiscardHandler))
	if err := d.Load(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids(d), ","); got != "a,b" {
		t.Errorf("valid flows: %s", got)
	}
	errs := map[string]string{}
	for _, e := range d.Entries() {
		errs[e.Source] = e.Error
	}
	if len(errs) != 5 || errs["a.yaml"] != "" || !strings.Contains(errs["dup.yaml"], `already defined in a.yaml`) ||
		errs["broken.yaml"] == "" || !strings.Contains(errs["unknown-step.yaml"], `unknown step "nope"`) {
		t.Errorf("entries: %v", errs)
	}
	if _, ok := d.Flow("c"); ok {
		t.Error("invalid flow must not be returned")
	}
}

func TestMissingDir(t *testing.T) {
	d := NewDir(filepath.Join(t.TempDir(), "nope"), nil, slog.New(slog.DiscardHandler))
	if err := d.Load(); err != nil || len(d.Flows()) != 0 {
		t.Errorf("err=%v flows=%d", err, len(d.Flows()))
	}
}

func TestWatchReloads(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.yaml", flowYAML("a"))
	d := NewDir(dir, nil, slog.New(slog.DiscardHandler))
	if err := d.Load(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Watch(ctx, 10*time.Millisecond)

	write(t, dir, "b.yaml", flowYAML("b"))
	_ = os.Remove(filepath.Join(dir, "a.yaml"))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Join(ids(d), ",") == "b" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("not reloaded: %v", ids(d))
}
