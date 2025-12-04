// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package importer

import (
	"math"

	ccunits "github.com/ClusterCockpit/cc-lib/ccUnits"
)

// getNormalizationFactor calculates the scaling factor needed to normalize a value
// to a more readable range (typically between 1.0 and 1000.0).
//
// For values greater than 1000, the function scales down by factors of 1000 (returns negative exponent).
// For values less than 1.0, the function scales up by factors of 1000 (returns positive exponent).
//
// Returns:
//   - factor: The multiplicative factor to apply (10^(count*scale))
//   - exponent: The power of 10 representing the adjustment (multiple of 3 for SI prefixes)
func getNormalizationFactor(v float64) (float64, int) {
	count := 0
	scale := -3

	if v > 1000.0 {
		for v > 1000.0 {
			v *= 1e-3
			count++
		}
	} else {
		for v < 1.0 {
			v *= 1e3
			count++
		}
		scale = 3
	}
	return math.Pow10(count * scale), count * scale
}

// getExponent calculates the SI prefix exponent from a numeric prefix value.
//
// For example:
//   - Input: 1000.0 (kilo) returns 3
//   - Input: 1000000.0 (mega) returns 6
//   - Input: 1000000000.0 (giga) returns 9
//
// Returns the exponent representing the power of 10 for the SI prefix.
func getExponent(p float64) int {
	count := 0

	for p > 1.0 {
		p = p / 1000.0
		count++
	}

	return count * 3
}

// newPrefixFromFactor computes a new SI unit prefix after applying a normalization factor.
//
// Given an original prefix and an exponent adjustment, this function calculates
// the resulting SI prefix. For example, if normalizing from bytes (no prefix) by
// a factor of 10^9, the result would be the "G" (giga) prefix.
//
// Parameters:
//   - op: The original SI prefix value
//   - e: The exponent adjustment to apply
//
// Returns the new SI prefix after adjustment.
func newPrefixFromFactor(op ccunits.Prefix, e int) ccunits.Prefix {
	f := float64(op)
	exp := math.Pow10(getExponent(f) - e)
	return ccunits.Prefix(exp)
}

// Normalize adjusts a metric value and its SI unit prefix to a more readable range.
//
// This function is useful for automatically scaling metrics to appropriate units.
// For example, normalizing 2048 MiB might result in ~2.0 GiB.
//
// The function analyzes the average value and determines if a different SI prefix
// would make the number more human-readable (typically keeping values between 1 and 1000).
//
// Parameters:
//   - avg: The metric value to normalize
//   - p: The current SI prefix as a string (e.g., "K", "M", "G")
//
// Returns:
//   - factor: The multiplicative factor to apply to convert the value
//   - newPrefix: The new SI prefix string to use
//
// Example:
//
//	factor, newPrefix := Normalize(2048.0, "M")  // returns factor for MB->GB conversion, "G"
func Normalize(avg float64, p string) (float64, string) {
	f, e := getNormalizationFactor(avg)

	if e != 0 {
		np := newPrefixFromFactor(ccunits.NewPrefix(p), e)
		return f, np.Prefix()
	}

	return f, p
}
