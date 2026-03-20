// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdispatch

import "math"

type ResamplePolicy string

const (
	ResamplePolicyLow    ResamplePolicy = "low"
	ResamplePolicyMedium ResamplePolicy = "medium"
	ResamplePolicyHigh   ResamplePolicy = "high"
)

// TargetPointsForPolicy returns the target number of data points for a given policy.
func TargetPointsForPolicy(policy ResamplePolicy) int {
	switch policy {
	case ResamplePolicyLow:
		return 200
	case ResamplePolicyMedium:
		return 500
	case ResamplePolicyHigh:
		return 1000
	default:
		return 0
	}
}

// ComputeResolution computes the resampling resolution in seconds for a given
// job duration, metric frequency, and target point count. Returns 0 if the
// total number of data points is already at or below targetPoints (no resampling needed).
func ComputeResolution(duration int64, frequency int64, targetPoints int) int {
	if frequency <= 0 || targetPoints <= 0 || duration <= 0 {
		return 0
	}

	totalPoints := duration / frequency
	if totalPoints <= int64(targetPoints) {
		return 0
	}

	targetRes := math.Ceil(float64(duration) / float64(targetPoints))
	// Round up to nearest multiple of frequency
	resolution := int(math.Ceil(targetRes/float64(frequency))) * int(frequency)

	return resolution
}
