// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the runtime configuration.
type Config struct {
	Listen   string   // address to listen on
	Upstream *url.URL // where proxied traffic goes (Immich or another proxy, e.g. upload optimizer)
	Immich   *url.URL // direct Immich URL for API calls made by immich-flows
	DataDir  string
	LogLevel slog.Level
	Workers  int    // concurrent flow jobs
	Secret   string // encrypts stored API keys; generated in DataDir if empty
	FlowsDir string // flow YAML files
	// TrashInterval is how often the trash is reconciled to detect deletions by Immich itself.
	TrashInterval time.Duration
	// HTTPAllow lists hosts the http step may call.
	HTTPAllow []string
}

// Load reads the configuration from the environment.
func Load() (*Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (*Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	c := &Config{
		Listen:  get("IMMICH_FLOWS_LISTEN", ":2283"),
		DataDir: get("IMMICH_FLOWS_DATA_DIR", "/data"),
		Secret:  get("IMMICH_FLOWS_SECRET", ""),
	}
	c.FlowsDir = get("IMMICH_FLOWS_FLOWS_DIR", filepath.Join(c.DataDir, "flows"))

	raw := get("IMMICH_FLOWS_UPSTREAM_URL", "")
	if raw == "" {
		return nil, errors.New("IMMICH_FLOWS_UPSTREAM_URL is required")
	}
	var err error
	if c.Upstream, err = parseURL("IMMICH_FLOWS_UPSTREAM_URL", raw); err != nil {
		return nil, err
	}
	if c.Immich, err = parseURL("IMMICH_FLOWS_IMMICH_URL", get("IMMICH_FLOWS_IMMICH_URL", raw)); err != nil {
		return nil, err
	}
	if c.Workers, err = strconv.Atoi(get("IMMICH_FLOWS_WORKERS", "4")); err != nil || c.Workers < 1 {
		return nil, fmt.Errorf("IMMICH_FLOWS_WORKERS: must be a positive integer")
	}
	if c.TrashInterval, err = time.ParseDuration(get("IMMICH_FLOWS_TRASH_INTERVAL", "10m")); err != nil || c.TrashInterval < time.Second {
		return nil, fmt.Errorf("IMMICH_FLOWS_TRASH_INTERVAL: must be a duration of at least 1s, e.g. 10m")
	}
	for h := range strings.SplitSeq(get("IMMICH_FLOWS_HTTP_ALLOW", ""), ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			c.HTTPAllow = append(c.HTTPAllow, h)
		}
	}
	if err := c.LogLevel.UnmarshalText([]byte(get("IMMICH_FLOWS_LOG_LEVEL", "info"))); err != nil {
		return nil, fmt.Errorf("IMMICH_FLOWS_LOG_LEVEL: %w", err)
	}
	return c, nil
}

func parseURL(key, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%s: invalid URL %q", key, raw)
	}
	return u, nil
}
