// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package logviewer

import (
	"testing"
	"time"
)

func TestParseSinceRelative(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 30, 0, 0, time.Local)

	tests := []struct {
		in   string
		want time.Time
	}{
		// The five presets offered by the frontend.
		{"15 min ago", now.Add(-15 * time.Minute)},
		{"1 hour ago", now.Add(-time.Hour)},
		{"6 hours ago", now.Add(-6 * time.Hour)},
		{"24 hours ago", now.Add(-24 * time.Hour)},
		{"7 days ago", now.AddDate(0, 0, -7)},

		{"now", now},
		{"30 s ago", now.Add(-30 * time.Second)},
		{"30 sec ago", now.Add(-30 * time.Second)},
		{"30 seconds ago", now.Add(-30 * time.Second)},
		{"2 m ago", now.Add(-2 * time.Minute)},
		{"2 minutes ago", now.Add(-2 * time.Minute)},
		{"3 h ago", now.Add(-3 * time.Hour)},
		{"1 day ago", now.AddDate(0, 0, -1)},
		{"2 weeks ago", now.AddDate(0, 0, -14)},
		{"1 month ago", now.AddDate(0, -1, 0)},
		{"1 year ago", now.AddDate(-1, 0, 0)},
		{"  1 Hour Ago  ", now.Add(-time.Hour)},
		{"0 hours ago", now},

		{"today", time.Date(2026, 3, 15, 0, 0, 0, 0, time.Local)},
		{"yesterday", time.Date(2026, 3, 14, 0, 0, 0, 0, time.Local)},

		{"2026-03-01", time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)},
		{"2026-03-01 08:15", time.Date(2026, 3, 1, 8, 15, 0, 0, time.Local)},
		{"2026-03-01 08:15:30", time.Date(2026, 3, 1, 8, 15, 30, 0, time.Local)},
		{"08:15", time.Date(2026, 3, 15, 8, 15, 0, 0, time.Local)},
		{"08:15:30", time.Date(2026, 3, 15, 8, 15, 30, 0, time.Local)},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseSince(tt.in, now)
			if err != nil {
				t.Fatalf("parseSince(%q) failed: %v", tt.in, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("parseSince(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseSinceRejects(t *testing.T) {
	now := time.Now()

	for _, in := range []string{
		"whenever",
		"ago",
		"many hours ago",
		"5 fortnights ago",
		"-5 hours ago",
		"2026-13-45",
	} {
		if _, err := parseSince(in, now); err == nil {
			t.Errorf("parseSince(%q) should have failed", in)
		}
	}
}
