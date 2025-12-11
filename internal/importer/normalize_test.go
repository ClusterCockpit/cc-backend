// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package importer

import (
	"fmt"
	"testing"

	ccunits "github.com/ClusterCockpit/cc-lib/ccUnits"
)

// TestNormalizeFactor tests the normalization of large byte values to gigabyte prefix.
// Verifies that values in the billions are correctly scaled to the "G" (giga) prefix.
func TestNormalizeFactor(t *testing.T) {
	// var us string
	s := []float64{2890031237, 23998994567, 389734042344, 390349424345}
	// r := []float64{3, 24, 390, 391}

	total := 0.0
	for _, number := range s {
		total += number
	}
	avg := total / float64(len(s))

	fmt.Printf("AVG: %e\n", avg)
	f, e := getNormalizationFactor(avg)

	fmt.Printf("Factor %e Count %d\n", f, e)

	np := ccunits.NewPrefix("")

	fmt.Printf("Prefix %e Short %s\n", float64(np), np.Prefix())

	p := newPrefixFromFactor(np, e)

	if p.Prefix() != "G" {
		t.Errorf("Failed Prefix or unit: Want G, Got %s", p.Prefix())
	}
}

// TestNormalizeKeep tests that values already in an appropriate range maintain their prefix.
// Verifies that when values don't require rescaling, the original "G" prefix is preserved.
func TestNormalizeKeep(t *testing.T) {
	s := []float64{3.0, 24.0, 390.0, 391.0}

	total := 0.0
	for _, number := range s {
		total += number
	}
	avg := total / float64(len(s))

	fmt.Printf("AVG: %e\n", avg)
	f, e := getNormalizationFactor(avg)

	fmt.Printf("Factor %e Count %d\n", f, e)

	np := ccunits.NewPrefix("G")

	fmt.Printf("Prefix %e Short %s\n", float64(np), np.Prefix())

	p := newPrefixFromFactor(np, e)

	if p.Prefix() != "G" {
		t.Errorf("Failed Prefix or unit: Want G, Got %s", p.Prefix())
	}
}
