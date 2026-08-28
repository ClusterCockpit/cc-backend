// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdispatch

import "testing"

func TestTargetPointsForPolicy(t *testing.T) {
	tests := []struct {
		policy ResamplePolicy
		want   int
	}{
		{ResamplePolicyLow, 200},
		{ResamplePolicyMedium, 500},
		{ResamplePolicyHigh, 1000},
		{ResamplePolicy("unknown"), 0},
		{ResamplePolicy(""), 0},
	}

	for _, tt := range tests {
		if got := TargetPointsForPolicy(tt.policy); got != tt.want {
			t.Errorf("TargetPointsForPolicy(%q) = %d, want %d", tt.policy, got, tt.want)
		}
	}
}

func TestComputeResolution(t *testing.T) {
	tests := []struct {
		name         string
		duration     int64
		frequency    int64
		targetPoints int
		want         int
	}{
		// 24h job, 60s frequency, 1440 total points
		{"low_24h_60s", 86400, 60, 200, 480},
		{"medium_24h_60s", 86400, 60, 500, 180},
		{"high_24h_60s", 86400, 60, 1000, 120},

		// 2h job, 60s frequency, 120 total points — no resampling needed
		{"low_2h_60s", 7200, 60, 200, 0},
		{"medium_2h_60s", 7200, 60, 500, 0},
		{"high_2h_60s", 7200, 60, 1000, 0},

		// Edge: zero/negative inputs
		{"zero_duration", 0, 60, 200, 0},
		{"zero_frequency", 86400, 0, 200, 0},
		{"zero_target", 86400, 60, 0, 0},
		{"negative_duration", -100, 60, 200, 0},

		// 12h job, 30s frequency, 1440 total points
		{"medium_12h_30s", 43200, 30, 500, 90},

		// Exact fit: total points == target points
		{"exact_fit", 12000, 60, 200, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeResolution(tt.duration, tt.frequency, tt.targetPoints)
			if got != tt.want {
				t.Errorf("ComputeResolution(%d, %d, %d) = %d, want %d",
					tt.duration, tt.frequency, tt.targetPoints, got, tt.want)
			}
		})
	}
}
