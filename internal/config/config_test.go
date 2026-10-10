package config

import (
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := load(env(map[string]string{"IMMICH_FLOWS_UPSTREAM_URL": "http://immich-server:2283"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":2283" || c.DataDir != "/data" || c.Workers != 4 || c.FlowsDir != "/data/flows" || c.TrashInterval != 10*time.Minute {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.Immich.String() != c.Upstream.String() {
		t.Errorf("immich URL should default to upstream, got %s", c.Immich)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]map[string]string{
		"missing upstream": {},
		"invalid upstream": {"IMMICH_FLOWS_UPSTREAM_URL": "immich-server"},
		"invalid workers":  {"IMMICH_FLOWS_UPSTREAM_URL": "http://a:1", "IMMICH_FLOWS_WORKERS": "0"},
		"invalid level":    {"IMMICH_FLOWS_UPSTREAM_URL": "http://a:1", "IMMICH_FLOWS_LOG_LEVEL": "loud"},
		"invalid interval": {"IMMICH_FLOWS_UPSTREAM_URL": "http://a:1", "IMMICH_FLOWS_TRASH_INTERVAL": "soon"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := load(env(m)); err == nil {
				t.Error("expected error")
			}
		})
	}
}
