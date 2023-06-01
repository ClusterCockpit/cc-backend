// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package importer

import (
	"math"

	ccunits "github.com/ClusterCockpit/cc-units"
)

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

func getExponent(p float64) int {
	count := 0

	for p > 1.0 {
		p = p / 1000.0
		count++
	}

	return count * 3
}

func newPrefixFromFactor(op ccunits.Prefix, e int) ccunits.Prefix {
	f := float64(op)
	exp := math.Pow10(getExponent(f) - e)
	return ccunits.Prefix(exp)
}

func Normalize(avg float64, p string) (float64, string) {
	f, e := getNormalizationFactor(avg)

	if e != 0 {
		np := newPrefixFromFactor(ccunits.NewPrefix(p), e)
		return f, np.Prefix()
	}

	return f, p
}
