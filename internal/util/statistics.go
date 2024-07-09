// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package util

import (
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"golang.org/x/exp/constraints"

	"fmt"
	"math"
	"sort"
)

func Min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func Max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func LoadJobStat(job *schema.JobMeta, metric string) float64 {
	if stats, ok := job.Statistics[metric]; ok {
		if metric == "mem_used" {
			return stats.Max
		} else {
			return stats.Avg
		}
	}

	return 0.0
}

func sortedCopy(input []float64) []float64 {
	sorted := make([]float64, len(input))
	copy(sorted, input)
	sort.Float64s(sorted)
	return sorted
}

func Mean(input []float64) (float64, error) {
	if len(input) == 0 {
		return math.NaN(), fmt.Errorf("input array is empty: %#v", input)
	}
	sum := 0.0
	for _, n := range input {
		sum += n
	}
	return sum / float64(len(input)), nil
}

func Median(input []float64) (median float64, err error) {
	c := sortedCopy(input)
	// Even numbers: add the two middle numbers, divide by two (use mean function)
	// Odd numbers: Use the middle number
	l := len(c)
	if l == 0 {
		return math.NaN(), fmt.Errorf("input array is empty: %#v", input)
	} else if l%2 == 0 {
		median, _ = Mean(c[l/2-1 : l/2+1])
	} else {
		median = c[l/2]
	}
	return median, nil
}
