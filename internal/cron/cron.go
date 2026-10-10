// Package cron parses 5-field cron expressions: minute hour day-of-month month day-of-week.
// Fields accept *, numbers, ranges (1-5), steps (*/15, 1-30/5) and lists (1,15). Shortcuts:
// @hourly, @daily, @weekly, @monthly, @yearly. Day-of-week 0 and 7 are Sunday. Names are not
// supported. As in standard cron, if both day fields are restricted, either may match.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed expression.
type Schedule struct {
	minute, hour, dom, month, dow uint64 // bit sets
	domAny, dowAny                bool
}

var shortcuts = map[string]string{
	"@hourly":  "0 * * * *",
	"@daily":   "0 0 * * *",
	"@weekly":  "0 0 * * 0",
	"@monthly": "0 0 1 * *",
	"@yearly":  "0 0 1 1 *",
}

// Parse parses an expression.
func Parse(expr string) (*Schedule, error) {
	expr = strings.TrimSpace(expr)
	if s, ok := shortcuts[expr]; ok {
		expr = s
	}
	f := strings.Fields(expr)
	if len(f) != 5 {
		return nil, fmt.Errorf("cron %q: want 5 fields (minute hour day month weekday)", expr)
	}
	var s Schedule
	var err error
	bounds := []struct {
		dst      *uint64
		min, max int
		name     string
	}{
		{&s.minute, 0, 59, "minute"}, {&s.hour, 0, 23, "hour"}, {&s.dom, 1, 31, "day of month"},
		{&s.month, 1, 12, "month"}, {&s.dow, 0, 7, "day of week"},
	}
	for i, b := range bounds {
		if *b.dst, err = field(f[i], b.min, b.max); err != nil {
			return nil, fmt.Errorf("cron %q: %s: %w", expr, b.name, err)
		}
	}
	if s.dow&(1<<7) != 0 {
		s.dow |= 1
	}
	s.domAny, s.dowAny = f[2] == "*", f[4] == "*"
	return &s, nil
}

func field(s string, lo, hi int) (uint64, error) {
	var set uint64
	for part := range strings.SplitSeq(s, ",") {
		rng, stepStr, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepStr)
			if err != nil || n < 1 {
				return 0, fmt.Errorf("invalid step %q", stepStr)
			}
			step = n
		}
		from, to := lo, hi
		switch {
		case rng == "*":
		case strings.Contains(rng, "-"):
			a, b, _ := strings.Cut(rng, "-")
			var err1, err2 error
			from, err1 = strconv.Atoi(a)
			to, err2 = strconv.Atoi(b)
			if err1 != nil || err2 != nil || from > to {
				return 0, fmt.Errorf("invalid range %q", rng)
			}
		default:
			n, err := strconv.Atoi(rng)
			if err != nil {
				return 0, fmt.Errorf("invalid value %q", rng)
			}
			from, to = n, n
			if hasStep {
				to = hi
			}
		}
		if from < lo || to > hi {
			return 0, fmt.Errorf("%q out of range %d-%d", part, lo, hi)
		}
		for v := from; v <= to; v += step {
			set |= 1 << uint(v)
		}
	}
	return set, nil
}

// Match reports whether t (truncated to the minute) is scheduled.
func (s *Schedule) Match(t time.Time) bool {
	if s.minute&(1<<uint(t.Minute())) == 0 || s.hour&(1<<uint(t.Hour())) == 0 || s.month&(1<<uint(t.Month())) == 0 {
		return false
	}
	dom := s.dom&(1<<uint(t.Day())) != 0
	dow := s.dow&(1<<uint(t.Weekday())) != 0
	switch {
	case s.domAny && s.dowAny:
		return true
	case s.domAny:
		return dow
	case s.dowAny:
		return dom
	default:
		return dom || dow
	}
}

// Next returns the first scheduled minute after t, or the zero time if none within 5 years.
func (s *Schedule) Next(t time.Time) time.Time {
	t = t.Truncate(time.Minute).Add(time.Minute)
	for end := t.AddDate(5, 0, 0); t.Before(end); t = t.Add(time.Minute) {
		if s.Match(t) {
			return t
		}
	}
	return time.Time{}
}
