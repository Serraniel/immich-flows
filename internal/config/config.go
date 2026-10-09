// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
)

// Config is the runtime configuration.
type Config struct {
	Listen   string   // address to listen on
	Upstream *url.URL // where proxied traffic goes (Immich or another proxy, e.g. upload optimizer)
	Immich   *url.URL // direct Immich URL for API calls made by immich-flows
	DataDir  string
	LogLevel slog.Level
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
	}

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
