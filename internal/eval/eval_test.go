package eval

import (
	"reflect"
	"strings"
	"testing"
)

var env = Env{
	"asset": map[string]any{
		"originalFileName": "SERVICE-A_123_cat_dog.jpg",
		"baseName":         "SERVICE-A_123_cat_dog",
		"type":             "IMAGE",
	},
	"steps": map[string]any{
		"a":             map[string]any{"tags": "cat_dog"},
		"hash-original": map[string]any{"md5": "abc"},
	},
}

func TestBool(t *testing.T) {
	cases := map[string]bool{
		``: true,
		`asset.originalFileName startsWith "SERVICE-A_"`: true,
		`asset.type == "VIDEO"`:                          false,
		`asset.baseName matches "^SERVICE-[AB]_"`:        true,
		`asset?.missing?.x == nil`:                       true,
	}
	for src, want := range cases {
		got, err := Bool(src, env)
		if err != nil || got != want {
			t.Errorf("%q: got %v, %v", src, got, err)
		}
	}
	if _, err := Bool(`asset.type`, env); err == nil {
		t.Error("non-bool must fail")
	}
}

func TestRender(t *testing.T) {
	in := map[string]any{
		"tags": []any{
			`hashes/MD5/{{ steps["hash-original"].md5 }}`,
			`{{ split(steps.a.tags, "_") | map("service-a/" + #) }}`,
		},
		"single": `{{ split(steps.a.tags, "_") }}`,
		"plain":  "no template",
		"num":    42,
		"map":    `{{ {"a": {"b": 1}}.a.b }}`,
		"quote":  `x{{ "}}" }}y`,
	}
	got, err := Render(in, env)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"tags":   []any{"hashes/MD5/abc", "service-a/cat", "service-a/dog"},
		"single": []any{"cat", "dog"},
		"plain":  "no template",
		"num":    42,
		"map":    1,
		"quote":  "x}}y",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("\ngot  %#v\nwant %#v", got, want)
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{`{{ x`, `{{ }}`, `{{ 1 + }}`} {
		if _, err := Render(src, env); err == nil {
			t.Errorf("%q: expected error", src)
		}
	}
	if err := CheckTemplates(map[string]any{"a": []any{"ok {{ 1 }}", "{{ (( }}"}}); err == nil ||
		!strings.Contains(err.Error(), "((") {
		t.Errorf("CheckTemplates: %v", err)
	}
	if err := CheckTemplates(map[string]any{"a": "{{ steps.x.y }}"}); err != nil {
		t.Errorf("valid template rejected: %v", err)
	}
}

func TestMaxNodes(t *testing.T) {
	src := strings.Repeat("1 + ", maxNodes) + "1"
	if err := Check(src); err == nil {
		t.Error("expected node limit error")
	}
}
