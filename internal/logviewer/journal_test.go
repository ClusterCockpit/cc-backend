// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package logviewer

import (
	"slices"
	"testing"
)

func TestJournalArgs(t *testing.T) {
	tests := []struct {
		name string
		q    Query
		want []string
	}{
		{
			name: "defaults",
			q:    Query{Since: "1 hour ago", Lines: 200, Level: -1},
			want: []string{
				"--output=json", "--no-pager", "-n", "200",
				"--since", "1 hour ago", "-u", "cc.service",
			},
		},
		{
			name: "level and search",
			q:    Query{Since: "15 min ago", Lines: 50, Level: 4, Search: "archive"},
			want: []string{
				"--output=json", "--no-pager", "-n", "50",
				"--since", "15 min ago", "-u", "cc.service",
				"--priority", "4", "--grep", "archive",
			},
		},
		{
			name: "level zero is not dropped",
			q:    Query{Since: "now", Lines: 1, Level: 0},
			want: []string{
				"--output=json", "--no-pager", "-n", "1",
				"--since", "now", "-u", "cc.service", "--priority", "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := journalArgs("cc.service", tt.q); !slices.Equal(got, tt.want) {
				t.Errorf("journalArgs() =\n%v\nwant\n%v", got, tt.want)
			}
		})
	}
}

func TestEntryFromJournal(t *testing.T) {
	entry := entryFromJournal(map[string]any{
		"PRIORITY":             "3",
		"MESSAGE":              "something failed",
		"__REALTIME_TIMESTAMP": "1773571200000000",
		"_SYSTEMD_UNIT":        "clustercockpit.service",
	})

	want := Entry{
		Timestamp: "1773571200000000",
		Priority:  3,
		Message:   "something failed",
		Unit:      "clustercockpit.service",
	}
	if entry != want {
		t.Errorf("entryFromJournal() = %+v, want %+v", entry, want)
	}

	// Numeric priorities and missing fields.
	entry = entryFromJournal(map[string]any{"PRIORITY": float64(4)})
	if entry.Priority != 4 || entry.Message != "" || entry.Unit != "" {
		t.Errorf("entryFromJournal() = %+v", entry)
	}

	// No priority at all falls back to info.
	if entry = entryFromJournal(map[string]any{}); entry.Priority != int(prioInfo) {
		t.Errorf("default priority = %d, want %d", entry.Priority, prioInfo)
	}
}
