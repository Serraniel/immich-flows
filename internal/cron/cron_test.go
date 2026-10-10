package cron

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestNext(t *testing.T) {
	for expr, c := range map[string]struct{ from, want string }{
		"*/15 * * * *":   {"2026-10-10 09:07", "2026-10-10 09:15"},
		"0 3 * * *":      {"2026-10-10 09:07", "2026-10-11 03:00"},
		"@daily":         {"2026-10-10 00:00", "2026-10-11 00:00"},
		"30 8 * * 1-5":   {"2026-10-10 09:00", "2026-10-12 08:30"}, // Saturday → Monday
		"0 0 * * 7":      {"2026-10-10 09:00", "2026-10-11 00:00"}, // 7 = Sunday
		"0 12 1,15 * *":  {"2026-10-02 00:00", "2026-10-15 12:00"},
		"0 0 29 2 *":     {"2026-01-01 00:00", "2028-02-29 00:00"},
		"0 0 13 * 5":     {"2026-10-14 00:00", "2026-10-16 00:00"}, // day 13 or Friday
		"5-10/5 0 * * *": {"2026-10-10 00:06", "2026-10-10 00:10"},
		"10/20 * * * *":  {"2026-10-10 00:31", "2026-10-10 00:50"},
	} {
		s, err := Parse(expr)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		if got := s.Next(at(c.from)); !got.Equal(at(c.want)) {
			t.Errorf("%s from %s: %s, want %s", expr, c.from, got.Format("2006-01-02 15:04"), c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, expr := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8", "5-1 * * * *", "*/0 * * * *", "a * * * *", "@sometimes"} {
		if _, err := Parse(expr); err == nil {
			t.Errorf("%q accepted", expr)
		}
	}
}
