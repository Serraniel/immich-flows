package flow

import (
	"strings"
	"testing"
)

func TestSetEnabled(t *testing.T) {
	src := "# keep me\nid: a\nname: A\ntrigger: {on: manual}\nsteps:\n  - use: log # inline\n    with: {message: hi}\n"
	out, err := SetEnabled([]byte(src), false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"# keep me", "# inline", "name: A\nenabled: false\ntrigger:"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	f, err := Parse(out)
	if err != nil || f.IsEnabled() {
		t.Fatalf("%v %v", f, err)
	}

	out, err = SetEnabled(out, true)
	if err != nil || strings.Count(string(out), "enabled:") != 1 || !strings.Contains(string(out), "enabled: true") {
		t.Errorf("%v\n%s", err, out)
	}

	if _, err := SetEnabled([]byte("- a"), true); err == nil {
		t.Error("sequence accepted")
	}
}
