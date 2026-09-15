// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package logviewer

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// absoluteLayouts are the absolute timestamp formats accepted by parseSince.
var absoluteLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"15:04:05",
	"15:04",
}

// parseSince converts the subset of journalctl's time syntax the log view uses
// into an absolute instant. Only the memory backend needs this; journal mode
// passes the string to journalctl unchanged.
//
// Accepted: "now", "today", "yesterday", "<N> <unit> ago" (seconds to years)
// and the absolute layouts above. Everything else is an error, so an
// unsupported expression is reported instead of silently returning the whole
// buffer.
func parseSince(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	switch s {
	case "", "now":
		return now, nil
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	case "yesterday":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -1), nil
	}

	if rel, ok := strings.CutSuffix(s, " ago"); ok {
		return parseRelative(strings.TrimSpace(rel), now)
	}

	for _, layout := range absoluteLayouts {
		if t, err := time.ParseInLocation(layout, s, now.Location()); err == nil {
			if layout == "15:04:05" || layout == "15:04" {
				// A bare time refers to today.
				return time.Date(now.Year(), now.Month(), now.Day(),
					t.Hour(), t.Minute(), t.Second(), 0, now.Location()), nil
			}
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("%w: cannot parse time expression '%s'", ErrInvalidQuery, s)
}

// parseRelative resolves "<N> <unit>" relative to now, going back in time.
func parseRelative(s string, now time.Time) (time.Time, error) {
	value, unit, found := strings.Cut(s, " ")
	if !found {
		return time.Time{}, fmt.Errorf("%w: cannot parse relative time '%s'", ErrInvalidQuery, s)
	}

	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return time.Time{}, fmt.Errorf("%w: invalid amount in relative time '%s'", ErrInvalidQuery, s)
	}

	unit = strings.TrimSuffix(strings.TrimSpace(unit), "s")
	switch unit {
	case "", "sec", "second":
		return now.Add(-time.Duration(n) * time.Second), nil
	case "m", "min", "minute":
		return now.Add(-time.Duration(n) * time.Minute), nil
	case "h", "hr", "hour":
		return now.Add(-time.Duration(n) * time.Hour), nil
	case "d", "day":
		return now.AddDate(0, 0, -n), nil
	case "w", "week":
		return now.AddDate(0, 0, -7*n), nil
	case "month":
		return now.AddDate(0, -n, 0), nil
	case "year":
		return now.AddDate(-n, 0, 0), nil
	}

	return time.Time{}, fmt.Errorf("%w: unknown time unit '%s'", ErrInvalidQuery, unit)
}
