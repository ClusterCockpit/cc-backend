// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package footprint builds job footprints and energy footprints from metric
// statistics. It is shared by the archiver and importer, which persist final
// values, and by the live computation for running jobs, so that both paths
// apply the same statistic selection, aggregation and rounding rules.
package footprint

import (
	"fmt"
	"math"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// Round rounds v to two decimal places, the precision used for all footprint
// and energy values.
func Round(v float64) float64 {
	return math.Round(v*100) / 100
}

// Fold combines per-host statistics into job-level statistics for the hosts of
// the job: avg is the mean of the host averages, min the smallest host minimum
// and max the largest host maximum. Hosts without data for a metric do not
// contribute to it, and a metric without data on any host is omitted.
//
// stats maps metric name to hostname to statistics, as returned by the metric
// data repositories.
func Fold(job *schema.Job, stats map[string]map[string]schema.MetricStatistics) map[string]schema.MetricStatistics {
	res := make(map[string]schema.MetricStatistics, len(stats))
	for metric, hosts := range stats {
		n := 0
		sum, min, max := 0.0, math.MaxFloat64, -math.MaxFloat64
		for _, r := range job.Resources {
			hs, ok := hosts[r.Hostname]
			if !ok {
				continue
			}
			n++
			sum += hs.Avg
			min = math.Min(min, hs.Min)
			max = math.Max(max, hs.Max)
		}
		if n == 0 {
			continue
		}
		res[metric] = schema.MetricStatistics{
			Avg: Round(sum / float64(n)),
			Min: Round(min),
			Max: Round(max),
		}
	}
	return res
}

// FromJobStatistics converts the per-metric statistics of an archived job into
// the input format of Build and BuildEnergy.
func FromJobStatistics(stats map[string]schema.JobStatistics) map[string]schema.MetricStatistics {
	res := make(map[string]schema.MetricStatistics, len(stats))
	for metric, s := range stats {
		res[metric] = schema.MetricStatistics{Avg: s.Avg, Min: s.Min, Max: s.Max}
	}
	return res
}

// StatType returns the footprint statistic ("avg", "min" or "max") configured
// for metric on the subcluster. The subcluster metric config already carries
// subcluster-specific overrides; the global metric list is only a fallback.
func StatType(sc *schema.SubCluster, metric string) (string, error) {
	var statType string
	if i, err := archive.MetricIndex(sc.MetricConfig, metric); err == nil {
		statType = sc.MetricConfig[i].Footprint
	}
	if statType == "" {
		for _, gm := range archive.GlobalMetricList {
			if gm.Name == metric {
				statType = gm.Footprint
				break
			}
		}
	}

	switch statType {
	case "avg", "min", "max":
		return statType, nil
	default:
		return "", fmt.Errorf("unknown footprint statType %q for metric %s", statType, metric)
	}
}

// Value selects the statistic named by statType.
func Value(s schema.MetricStatistics, statType string) float64 {
	switch statType {
	case "min":
		return s.Min
	case "max":
		return s.Max
	default:
		return s.Avg
	}
}

// Build returns the footprint of a job on subcluster sc as a map keyed
// "<metric>_<stat>". Metrics without statistics are reported as 0 when
// zeroMissing is set (the persisted archive format) and omitted otherwise.
func Build(sc *schema.SubCluster, stats map[string]schema.MetricStatistics, zeroMissing bool) (map[string]float64, error) {
	fp := make(map[string]float64, len(sc.Footprint))
	for _, metric := range sc.Footprint {
		statType, err := StatType(sc, metric)
		if err != nil {
			return nil, err
		}

		s, ok := stats[metric]
		if !ok && !zeroMissing {
			continue
		}
		fp[fmt.Sprintf("%s_%s", metric, statType)] = Value(s, statType)
	}
	return fp, nil
}

// BuildEnergy returns the energy footprint (kWh per energy metric) and the total
// energy of a job on subcluster sc that ran on numNodes nodes for durationSec
// seconds. For metrics configured as "power" the energy is
// avg power per node * nodes * hours / 1000. Metrics configured as "energy" are
// not supported yet and report 0.
func BuildEnergy(
	sc *schema.SubCluster,
	stats map[string]schema.MetricStatistics,
	numNodes int32,
	durationSec int32,
) (map[string]float64, float64) {
	efp := make(map[string]float64, len(sc.EnergyFootprint))
	total := 0.0
	for _, metric := range sc.EnergyFootprint {
		energy := 0.0
		if i, err := archive.MetricIndex(sc.MetricConfig, metric); err == nil {
			switch sc.MetricConfig[i].Energy {
			case "energy":
				// FIXME: Needs sum as stats type to accumulate energy values over time
				cclog.Debugf("energy footprint for metric %s: energy type 'energy' is not implemented, reporting 0", metric)
			case "power":
				// Energy (kWh) = Power (W) * Time (h) / 1000; for shared jobs the node
				// average already reflects the partial resources and numNodes is 1.
				energy = Round(stats[metric].Avg * float64(numNodes) * (float64(durationSec) / 3600.0) / 1000.0)
			}
		} else {
			cclog.Warnf("energy footprint: unknown metric %s on subcluster %s", metric, sc.Name)
		}

		efp[metric] = energy
		total += energy
	}
	return efp, Round(total)
}
